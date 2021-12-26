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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

type balancerBuilder struct {
}

// Build 创建一个Balancer
func (bb *balancerBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	return &polarisNamingBalancer{
		cc:       cc,
		target:   opts.Target,
		subConns: make(map[resolver.Address]balancer.SubConn),
		scStates: make(map[balancer.SubConn]connectivity.State),
		csEvltr:  &balancer.ConnectivityStateEvaluator{},
	}
}

// Name 返回Name
func (bb *balancerBuilder) Name() string {
	return scheme
}

type polarisNamingBalancer struct {
	cc      balancer.ClientConn
	target  resolver.Target
	rwMutex sync.RWMutex

	csEvltr *balancer.ConnectivityStateEvaluator
	state   connectivity.State

	subConns map[resolver.Address]balancer.SubConn
	scStates map[balancer.SubConn]connectivity.State

	v2Picker    balancer.V2Picker
	consumerAPI api.ConsumerAPI

	options *dialOptions

	resolverErr error // the last error reported by the resolver; cleared on successful resolution
	connErr     error // the last connection error; cleared upon leaving TransientFailure
}

// of sc has changed.
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

// HandleResolvedAddrs is called by gRPC to send updated resolved addresses to
// balancers.
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

func (p *polarisNamingBalancer) createSubConnection(addr resolver.Address) {
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	if _, ok := p.subConns[addr]; !ok {
		// a is a new address (not existing in b.subConns).
		sc, err := p.cc.NewSubConn([]resolver.Address{addr}, balancer.NewSubConnOptions{HealthCheckEnabled: true})
		if err != nil {
			grpclog.Warningf("[Polaris]balancer: failed to create new SubConn: %v", err)
			return
		}
		p.subConns[addr] = sc
		p.scStates[sc] = connectivity.Idle
		sc.Connect()
	}
}

// UpdateClientConnState is called by gRPC when the state of the ClientConn
// changes.  If the error returned is ErrBadResolverState, the ClientConn
// will begin calling ResolveNow on the active name resolver with
// exponential backoff until a subsequent call to UpdateClientConnState
// returns a nil error.  Any other errors are currently ignored.
func (p *polarisNamingBalancer) UpdateClientConnState(state balancer.ClientConnState) error {
	if grpclog.V(2) {
		grpclog.Infoln("[Polaris]balancer: got new ClientConn state: ", state)
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
		p.consumerAPI = api.NewConsumerAPIByContext(sdkCtx)
	}
	// Successful resolution; clear resolver error and ensure we return nil.
	p.resolverErr = nil
	// addrsSet is the set converted from addrs, it's used for quick lookup of an address.
	addrsSet := make(map[resolver.Address]struct{})
	for _, a := range state.ResolverState.Addresses {
		if nil == p.options {
			p.options = a.Attributes.Value(keyDialOptions).(*dialOptions)
		}
		addrsSet[a] = struct{}{}
		p.createSubConnection(a)
	}
	p.rwMutex.Lock()
	defer p.rwMutex.Unlock()
	for a, sc := range p.subConns {
		// a was removed by resolver.
		if _, ok := addrsSet[a]; !ok {
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

// UpdateSubConnState is called by gRPC when the state of a SubConn
// changes.
func (p *polarisNamingBalancer) UpdateSubConnState(sc balancer.SubConn, state balancer.SubConnState) {
	s := state.ConnectivityState
	if grpclog.V(2) {
		grpclog.Infof("base.baseBalancer: handle SubConn state change: %p, %v", sc, s)
	}
	oldS, quit := func() (connectivity.State, bool) {
		p.rwMutex.Lock()
		defer p.rwMutex.Unlock()
		oldS, ok := p.scStates[sc]
		if !ok {
			if grpclog.V(2) {
				grpclog.Infof("base.baseBalancer: got state changes for an unknown SubConn: %p, %v", sc, s)
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

// regeneratePicker takes a snapshot of the balancer, and generates a picker
// from it. The picker is
//  - errPicker if the balancer is in TransientFailure,
//  - built by the pickerBuilder with all READY SubConns otherwise.
func (p *polarisNamingBalancer) regeneratePicker(options *dialOptions) {
	if p.state == connectivity.TransientFailure {
		p.v2Picker = base.NewErrPickerV2(balancer.TransientFailureError(p.mergeErrors()))
		return
	}
	readySCs := make(map[string]balancer.SubConn)
	p.rwMutex.RLock()
	defer p.rwMutex.RUnlock()
	// Filter out all ready SCs from full subConn map.
	for addr, sc := range p.subConns {
		if st, ok := p.scStates[sc]; ok && st == connectivity.Ready {
			readySCs[addr.Addr] = sc
		}
	}
	p.v2Picker = &polarisNamingPicker{
		balancer: p,
		readySCs: readySCs,
		options:  options,
	}
}

// mergeErrors builds an error from the last connection error and the last
// resolver error.  Must only be called if b.state is TransientFailure.
func (p *polarisNamingBalancer) mergeErrors() error {
	// connErr must always be non-nil unless there are no SubConns, in which
	// case resolverErr must be non-nil.
	if p.connErr == nil {
		return fmt.Errorf("last resolver error: %v", p.resolverErr)
	}
	if p.resolverErr == nil {
		return fmt.Errorf("last connection error: %v", p.connErr)
	}
	return fmt.Errorf("last connection error: %v; last resolver error: %v", p.connErr, p.resolverErr)
}

type polarisNamingPicker struct {
	balancer *polarisNamingBalancer
	readySCs map[string]balancer.SubConn
	options  *dialOptions
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
// Pick should not block.  If the balancer needs to do I/O or any blocking
// or time-consuming work to service this call, it should return
// ErrNoSubConnAvailable, and the Pick call will be repeated by gRPC when
// the Picker is updated (using ClientConn.UpdateState).
//
// If an error is returned:
//
// - If the error is ErrNoSubConnAvailable, gRPC will block until a new
//   Picker is provided by the balancer (using ClientConn.UpdateState).
//
// - If the error implements IsTransientFailure() bool, returning true,
//   wait for ready RPCs will wait, but non-wait for ready RPCs will be
//   terminated with this error's Error() string and status code
//   Unavailable.
//
// - Any other errors terminate all RPCs with the code and message
//   provided.  If the error is not a status error, it will be converted by
//   gRPC to a status error with code Unknown.
func (pnp *polarisNamingPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	request := &api.GetOneInstanceRequest{}
	request.Namespace = getNamespace(pnp.options)
	request.Service = pnp.balancer.target.Authority
	if len(pnp.options.DstMetadata) > 0 {
		request.Metadata = pnp.options.DstMetadata
	}
	sourceService := buildSourceInfo(pnp.options)
	if sourceService != nil {
		// 如果在Conf中配置了SourceService，则优先使用配置
		request.SourceService = sourceService
	} else {
		// 如果没有配置，则使用gRPC Header作为规则路由元数据
		if md, ok := metadata.FromOutgoingContext(info.Ctx); ok {
			mp := make(map[string]string)
			for kvs := range md {
				if pnp.options.HeaderPrefix != nil {
					// 如果配置的前缀不为空，则要求header匹配前缀
					for _, prefix := range pnp.options.HeaderPrefix {
						if strings.HasPrefix(kvs, prefix) {
							mp[strings.TrimPrefix(kvs, prefix)] = md[kvs][0]
							break
						}
					}
				} else {
					// 否则全量使用header作为规则路由元数据
					mp[kvs] = md[kvs][0]
				}
			}
			if len(mp) > 0 {
				request.SourceService = &model.ServiceInfo{
					Metadata: mp,
				}
			}
		}
	}
	resp, err := pnp.balancer.consumerAPI.GetOneInstance(request)
	if nil != err {
		return balancer.PickResult{}, err
	}
	targetInstance := resp.GetInstance()
	addr := fmt.Sprintf("%s:%d", targetInstance.GetHost(), targetInstance.GetPort())
	subSc, ok := pnp.readySCs[addr]
	if ok {
		reporter := &resultReporter{
			instance: targetInstance, consumerAPI: pnp.balancer.consumerAPI, startTime: time.Now()}
		return balancer.PickResult{
			SubConn: subSc,
			Done:    reporter.report,
		}, nil
	}
	pnp.balancer.createSubConnection(resolver.Address{Addr: addr})
	return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
}

type resultReporter struct {
	instance    model.Instance
	consumerAPI api.ConsumerAPI
	startTime   time.Time
}

func (r *resultReporter) report(info balancer.DoneInfo) {
	recvErr := info.Err
	if !info.BytesReceived {
		return
	}
	callResult := &api.ServiceCallResult{}
	callResult.CalledInstance = r.instance
	var code uint32
	if nil != recvErr {
		callResult.RetStatus = api.RetFail
		st, _ := status.FromError(recvErr)
		code = uint32(st.Code())
	} else {
		callResult.RetStatus = api.RetSuccess
		code = 0
	}
	callResult.SetDelay(time.Now().Sub(r.startTime))
	callResult.SetRetCode(int32(code))
	_ = r.consumerAPI.UpdateServiceCallResult(callResult)
}
