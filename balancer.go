/**
 * Tencent is pleased to support the open source community by making Polaris available.
 *
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 *
 * Licensed under the BSD 3-Clause License (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://opensource.org/licenses/BSD-3-Clause
 *
 * Unless required by applicable law or agreed to in writing, software distributed
 * under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
 * CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 */

package grpcpolaris

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/polarismesh/polaris-go"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/specification/source/go/api/v1/traffic_manage"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"
	"google.golang.org/grpc/status"
)

var (
	reportInfoAnalyzer ReportInfoAnalyzer = func(info balancer.DoneInfo) (model.RetStatus, uint32) {
		recErr := info.Err
		if nil != recErr {
			st, _ := status.FromError(recErr)
			code := uint32(st.Code())
			return api.RetFail, code
		}
		return api.RetSuccess, 0
	}
)

var (
	ErrorPolarisServiceRouteRuleEmpty = errors.New("service route rule is empty")
)

// SetReportInfoAnalyzer sets report info analyzer
func SetReportInfoAnalyzer(analyzer ReportInfoAnalyzer) {
	reportInfoAnalyzer = analyzer
}

type (
	balancerBuilder struct {
	}

	// ReportInfoAnalyzer analyze balancer.DoneInfo to polaris report info
	ReportInfoAnalyzer func(info balancer.DoneInfo) (model.RetStatus, uint32)
)

// Build creates polaris balancer.Balancer implement
func (bb *balancerBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	grpclog.Infof("[Polaris][Balancer] start to build polaris balancer")
	target := opts.Target
	host, _, err := parseHost(target.URL.Host)
	if err != nil {
		grpclog.Errorln("[Polaris][Balancer] failed to create balancer: " + err.Error())
		return nil
	}
	return &polarisNamingBalancer{
		cc:       cc,
		target:   opts.Target,
		host:     host,
		subConns: make(map[string]balancer.SubConn),
		scStates: make(map[balancer.SubConn]connectivity.State),
		csEvltr:  &balancer.ConnectivityStateEvaluator{},
	}
}

// Name return name
func (bb *balancerBuilder) Name() string {
	return scheme
}

// ParseConfig parses the JSON load balancer config provided into an
// internal form or returns an error if the config is invalid.  For future
// compatibility reasons, unknown fields in the config should be ignored.
func (bb *balancerBuilder) ParseConfig(cfgStr json.RawMessage) (serviceconfig.LoadBalancingConfig, error) {
	cfg := &LBConfig{}
	if err := json.Unmarshal(cfgStr, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

type polarisNamingBalancer struct {
	cc      balancer.ClientConn
	target  resolver.Target
	host    string
	rwMutex sync.RWMutex

	csEvltr *balancer.ConnectivityStateEvaluator
	state   connectivity.State

	subConns map[string]balancer.SubConn
	scStates map[balancer.SubConn]connectivity.State

	v2Picker    balancer.Picker
	consumerAPI polaris.ConsumerAPI
	routerAPI   polaris.RouterAPI

	lbCfg *LBConfig

	options  *dialOptions
	response *model.InstancesResponse

	resolverErr error // the last error reported by the resolver; cleared on successful resolution
	connErr     error // the last connection error; cleared upon leaving TransientFailure
}

// HandleSubConnStateChange .is called by gRPC of sc has changed.
// Balancer is expected to aggregate all the state of SubConn and report
// that back to gRPC.
// Balancer should also generate and update Pickers when its internal state has
// been changed by the new state.
//
// Deprecated: if V2Balancer is implemented by the Balancer,
// UpdateSubConnState will be called instead.
func (p *polarisNamingBalancer) HandleSubConnStateChange(sc balancer.SubConn, state connectivity.State) {
	panic("not implemented")
}

// HandleResolvedAddrs is called by gRPC to send updated resolved addresses to balancers.
// Balancer can create new SubConn or remove SubConn with the addresses.
// An empty address slice and a non-nil error will be passed if the resolver returns
// non-nil error to gRPC.
//
// Deprecated: if V2Balancer is implemented by the Balancer,
// UpdateClientConnState will be called instead.
func (p *polarisNamingBalancer) HandleResolvedAddrs([]resolver.Address, error) {
	panic("not implemented")
}

// Close closes the balancer. The balancer is not required to call
// ClientConn.RemoveSubConn for its existing SubConns.
func (p *polarisNamingBalancer) Close() {

}

func buildAddressKey(addr resolver.Address) string {
	return fmt.Sprintf("%s", addr.Addr)
}

func (p *polarisNamingBalancer) createSubConnection(addr resolver.Address) {
	key := buildAddressKey(addr)
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	if _, ok := p.subConns[key]; ok {
		return
	}
	// is a new address (not existing in b.subConns).
	sc, err := p.cc.NewSubConn([]resolver.Address{addr}, balancer.NewSubConnOptions{HealthCheckEnabled: true})
	if err != nil {
		grpclog.Warningf("[Polaris][Balancer] failed to create new SubConn: %v", err)
		return
	}
	p.subConns[key] = sc
	p.scStates[sc] = connectivity.Idle
	sc.Connect()
}

// UpdateClientConnState is called by gRPC when the state of the ClientConn changes.
// If the error returned is ErrBadResolverState, the ClientConn
// will begin calling ResolveNow on the active name resolver with
// exponential backoff until a subsequent call to UpdateClientConnState returns a nil error.
// Any other errors are currently ignored.
func (p *polarisNamingBalancer) UpdateClientConnState(state balancer.ClientConnState) error {
	if nil == p.options && nil != state.ResolverState.Attributes {
		p.options = state.ResolverState.Attributes.Value(keyDialOptions).(*dialOptions)
	}
	if nil != state.ResolverState.Attributes {
		p.response = state.ResolverState.Attributes.Value(keyResponse).(*model.InstancesResponse)
	}

	if state.BalancerConfig != nil {
		p.lbCfg = state.BalancerConfig.(*LBConfig)
	}
	if grpclog.V(2) {
		grpclog.Infoln("[Polaris][Balancer] got new ClientConn state: ", state)
	}
	if len(state.ResolverState.Addresses) == 0 {
		p.ResolverError(errors.New("produced zero addresses"))
		return balancer.ErrBadResolverState
	}
	if nil == p.consumerAPI {
		sdkCtx, err := PolarisContext()
		if nil != err {
			return err
		}
		p.consumerAPI = polaris.NewConsumerAPIByContext(sdkCtx)
		p.routerAPI = polaris.NewRouterAPIByContext(sdkCtx)
	}
	// Successful resolution; clear resolver error and ensure we return nil.
	p.resolverErr = nil
	// addressSet is the set converted from address;
	// it's used for a quick lookup of an address.
	addressSet := make(map[string]struct{})
	for _, a := range state.ResolverState.Addresses {
		addressSet[buildAddressKey(a)] = struct{}{}
		p.createSubConnection(a)
	}
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	for a, sc := range p.subConns {
		// a way removed by resolver.
		if _, ok := addressSet[a]; !ok {
			p.cc.RemoveSubConn(sc)
			delete(p.subConns, a)
			// Keep the state of this sc in b.scStates until sc's state becomes Shutdown.
			// The entry will be deleted in HandleSubConnStateChange.
		}
	}
	return nil
}

// ResolverError is called by gRPC when the name resolver reports an error.
func (p *polarisNamingBalancer) ResolverError(err error) {
	p.resolverErr = err
	if len(p.subConns) == 0 {
		p.state = connectivity.TransientFailure
	}
	if p.state != connectivity.TransientFailure {
		// The picker will not change since the balancer does not currently
		// report an error.
		return
	}
	p.regeneratePicker(nil)
	p.cc.UpdateState(balancer.State{
		ConnectivityState: p.state,
		Picker:            p.v2Picker,
	})
}

// UpdateSubConnState is called by gRPC when the state of a SubConn changes.
func (p *polarisNamingBalancer) UpdateSubConnState(sc balancer.SubConn, state balancer.SubConnState) {
	s := state.ConnectivityState
	if grpclog.V(2) {
		grpclog.Infof("[Polaris][Balancer] handle SubConn state change: %p, %v", sc, s)
	}
	oldS, quit := func() (connectivity.State, bool) {
		p.rwMutex.Lock()
		defer p.rwMutex.Unlock()
		oldS, ok := p.scStates[sc]
		if !ok {
			if grpclog.V(2) {
				grpclog.Infof("[Polaris][Balancer] got state changes for an unknown SubConn: %p, %v", sc, s)
			}
			return connectivity.TransientFailure, true
		}
		if oldS == connectivity.TransientFailure && s == connectivity.Connecting {
			// Once a subconn enters TRANSIENT_FAILURE, ignore subsequent
			// CONNECTING transitions to prevent the aggregated state from being
			// always CONNECTING when many backends exist but are all down.
			return oldS, true
		}
		p.scStates[sc] = s
		switch s {
		case connectivity.Idle:
			sc.Connect()
		case connectivity.Shutdown:
			// When an address was removed by resolver, b called RemoveSubConn but
			// kept the sc's state in scStates. Remove state for this sc here.
			delete(p.scStates, sc)
		case connectivity.TransientFailure:
			// Save error to be reported via picker.
			p.connErr = state.ConnectionError
		}
		return oldS, false
	}()
	if quit {
		return
	}
	p.state = p.csEvltr.RecordTransition(oldS, s)

	// Regenerate picker when one of the following happens:
	//  - this sc entered or left ready
	//  - the aggregated state of balancer is TransientFailure
	//    (may need to update error message)
	if (s == connectivity.Ready) != (oldS == connectivity.Ready) ||
		p.state == connectivity.TransientFailure {
		p.regeneratePicker(p.options)
	}

	p.cc.UpdateState(balancer.State{ConnectivityState: p.state, Picker: p.v2Picker})
}

// regeneratePicker takes a snapshot of the balancer, and generates a picker from it.
// The picker is errPicker if the balancer is in TransientFailure,
// built by the pickerBuilder with all READY SubConns otherwise.
func (p *polarisNamingBalancer) regeneratePicker(options *dialOptions) {
	if p.state == connectivity.TransientFailure {
		p.v2Picker = base.NewErrPicker(p.mergeErrors())
		return
	}
	readySCs := make(map[string]balancer.SubConn)
	p.rwMutex.RLock()
	defer p.rwMutex.RUnlock()
	// Filter out all ready SCs from full subConn map.
	for addr, sc := range p.subConns {
		if st, ok := p.scStates[sc]; ok && st == connectivity.Ready {
			readySCs[addr] = sc
		}
	}

	totalWeight := 0
	readyInstances := make([]model.Instance, 0, len(readySCs))
	copyR := *p.response
	for _, instance := range copyR.Instances {
		// see buildAddressKey
		key := instance.GetHost() + ":" + strconv.FormatInt(int64(instance.GetPort()), 10)
		if _, ok := readySCs[key]; ok {
			readyInstances = append(readyInstances, instance)
			totalWeight += instance.GetWeight()
		}
	}
	copyR.Instances = readyInstances
	copyR.TotalWeight = totalWeight
	picker := &polarisNamingPicker{
		balancer: p,
		readySCs: readySCs,
		options:  options,
		lbCfg:    p.lbCfg,
		response: &copyR,
	}
	p.v2Picker = picker
}

// mergeErrors builds an error from the last connection error and the last resolver error.
// It Must only be called if the b.state is TransientFailure.
func (p *polarisNamingBalancer) mergeErrors() error {
	// connErr must always be non-nil unless there are no SubConns, in which
	// case resolverErr must be non-nil.
	if p.connErr == nil {
		return fmt.Errorf("last resolver error: %w", p.resolverErr)
	}
	if p.resolverErr == nil {
		return fmt.Errorf("last connection error: %w", p.connErr)
	}
	return fmt.Errorf("last connection error: %v; last resolver error: %v", p.connErr, p.resolverErr)
}

type polarisNamingPicker struct {
	balancer *polarisNamingBalancer
	readySCs map[string]balancer.SubConn
	options  *dialOptions
	lbCfg    *LBConfig
	response *model.InstancesResponse
}

func buildSourceInfo(options *dialOptions) *model.ServiceInfo {
	var valueSet bool
	svcInfo := &model.ServiceInfo{}
	if len(options.SrcService) > 0 {
		svcInfo.Service = options.SrcService
		valueSet = true
	}
	if len(options.SrcMetadata) > 0 {
		svcInfo.Metadata = options.SrcMetadata
		valueSet = true
	}
	if valueSet {
		svcInfo.Namespace = getNamespace(options)
		return svcInfo
	}
	return nil
}

// Pick returns the connection to use for this RPC and related information.
//
// Pick should not block.
// If the balancer needs to do I/O or any blocking
// or time-consuming work to service this call, it should return to ErrNoSubConnAvailable,
// and the Pick call will be repeated by gRPC when
// the Picker is updated (using ClientConn.UpdateState).
//
// If an error is returned:
//
//	If the error is ErrNoSubConnAvailable, gRPC will block until a new
//	Picker is provided by the balancer (using ClientConn.UpdateState).
//
//	If the error implements IsTransientFailure() bool, returning true,
//	wait for ready RPCs will wait, but non-wait for ready RPCs will be
//	terminated with this error's Error() string and status code Unavailable.
//
//	Any other errors terminate all RPCs with the code and message provided.
//	If the error is not a status error, it will be converted by
//	gRPC to a status error with code Unknown.
func (pnp *polarisNamingPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	var resp *model.InstancesResponse
	sourceService := buildSourceInfo(pnp.options)

	if pnp.options.Route {
		request := &polaris.ProcessRoutersRequest{}
		request.DstInstances = pnp.response
		if sourceService != nil {
			// 如果在Conf中配置了SourceService，则优先使用配置
			request.SourceService = *sourceService
		} else {
			if err := pnp.addTrafficLabels(info, request); err != nil {
				grpclog.Errorf("[Polaris][Balancer] fetch traffic labels fail : %+v", err)
			}
		}

		if grpclog.V(2) {
			grpclog.Infof("[Polaris][Balancer] get one instance request : %+v", request)
		}
		var err error
		resp, err = pnp.balancer.routerAPI.ProcessRouters(request)
		if err != nil {
			return balancer.PickResult{}, err
		}
	} else {
		resp = pnp.response
	}

	lbReq := pnp.buildLoadBalanceRequest(info, resp)
	oneInsResp, err := pnp.balancer.routerAPI.ProcessLoadBalance(lbReq)
	if nil != err {
		return balancer.PickResult{}, err
	}
	targetInstance := oneInsResp.GetInstance()
	addr := fmt.Sprintf("%s:%d", targetInstance.GetHost(), targetInstance.GetPort())
	subSc, ok := pnp.readySCs[addr]
	if ok {
		reporter := &resultReporter{
			instance:      targetInstance,
			consumerAPI:   pnp.balancer.consumerAPI,
			startTime:     time.Now(),
			sourceService: sourceService,
		}

		return balancer.PickResult{
			SubConn: subSc,
			Done:    reporter.report,
		}, nil
	}

	return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
}

func (pnp *polarisNamingPicker) buildLoadBalanceRequest(info balancer.PickInfo,
	destIns model.ServiceInstances) *polaris.ProcessLoadBalanceRequest {
	lbReq := &polaris.ProcessLoadBalanceRequest{
		ProcessLoadBalanceRequest: model.ProcessLoadBalanceRequest{
			DstInstances: destIns,
		},
	}
	if pnp.lbCfg != nil {
		if pnp.lbCfg.LbPolicy != "" {
			lbReq.LbPolicy = pnp.lbCfg.LbPolicy
		}
		if pnp.lbCfg.HashKey != "" {
			lbReq.HashKey = []byte(pnp.lbCfg.HashKey)
		}
	}

	// if request scope set Lb Info, use first
	md, ok := metadata.FromOutgoingContext(info.Ctx)
	if ok {
		lbPolicyValues := md.Get(polarisRequestLbPolicy)
		lbHashKeyValues := md.Get(polarisRequestLbHashKey)

		if len(lbPolicyValues) > 0 && len(lbHashKeyValues) > 0 {
			lbReq.LbPolicy = lbPolicyValues[0]
			lbReq.HashKey = []byte(lbHashKeyValues[0])
		}
	}

	return lbReq
}

func (pnp *polarisNamingPicker) addTrafficLabels(info balancer.PickInfo, insReq *polaris.ProcessRoutersRequest) error {
	req := &model.GetServiceRuleRequest{}
	req.Namespace = getNamespace(pnp.options)
	req.Service = pnp.balancer.host
	req.SetTimeout(time.Second)
	engine := pnp.balancer.consumerAPI.SDKContext().GetEngine()
	resp, err := engine.SyncGetServiceRule(model.EventRouting, req)
	if err != nil {
		grpclog.Errorf("[Polaris][Balancer] ns:%s svc:%s get route rule fail : %+v",
			req.GetNamespace(), req.GetService(), err)
		return err
	}

	if resp == nil || resp.GetValue() == nil {
		grpclog.Errorf("[Polaris][Balancer] ns:%s svc:%s get route rule empty", req.GetNamespace(), req.GetService())
		return ErrorPolarisServiceRouteRuleEmpty
	}

	routeRule := resp.GetValue().(*traffic_manage.Routing)
	labels := make([]string, 0, 4)
	labels = append(labels, collectRouteLabels(routeRule.GetInbounds())...)
	labels = append(labels, collectRouteLabels(routeRule.GetInbounds())...)

	header, ok := metadata.FromOutgoingContext(info.Ctx)
	if !ok {
		header = metadata.MD{}
	}
	for i := range labels {
		label := labels[i]
		if strings.Compare(label, model.LabelKeyPath) == 0 {
			insReq.AddArguments(model.BuildPathArgument(extractBareMethodName(info.FullMethodName)))
			continue
		}
		if strings.HasPrefix(label, model.LabelKeyHeader) {
			values := header.Get(strings.TrimPrefix(label, model.LabelKeyHeader))
			if len(values) > 0 {
				insReq.AddArguments(model.BuildArgumentFromLabel(label, fmt.Sprintf("%+v", values[0])))
			}
		}
	}

	return nil
}

func collectRouteLabels(routings []*traffic_manage.Route) []string {
	ret := make([]string, 0, 4)

	for i := range routings {
		route := routings[i]
		sources := route.GetSources()
		for p := range sources {
			source := sources[p]
			for k := range source.GetMetadata() {
				ret = append(ret, k)
			}
		}
	}

	return ret
}

type resultReporter struct {
	instance      model.Instance
	consumerAPI   polaris.ConsumerAPI
	startTime     time.Time
	sourceService *model.ServiceInfo
}

func (r *resultReporter) report(info balancer.DoneInfo) {
	if !info.BytesReceived {
		return
	}
	retStatus, code := reportInfoAnalyzer(info)

	callResult := &polaris.ServiceCallResult{}
	callResult.CalledInstance = r.instance
	callResult.RetStatus = retStatus
	callResult.SourceService = r.sourceService
	callResult.SetDelay(time.Since(r.startTime))
	callResult.SetRetCode(int32(code))
	if err := r.consumerAPI.UpdateServiceCallResult(callResult); err != nil {
		grpclog.Errorf("[Polaris][Balancer] report grpc call info fail : %+v", err)
	}
}
