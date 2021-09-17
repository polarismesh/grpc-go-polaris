/**
 * Tencent is pleased to support the open source community by making CL5 available.
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
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/balancer"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
)

// Name
const Name = "polaris"

func init() {
	balancer.Register(newBalancerBuilder())
}

// newBalancerBuilder
func newBalancerBuilder() balancer.Builder {
	return &polarisBalancerBuilder{}
}

// polarisBalancerBuilder
type polarisBalancerBuilder struct {
	polarisConsumer api.ConsumerAPI
}

// Build 创建一个Balancer
func (pbb *polarisBalancerBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	return &polarisBalancer{
		cc:              cc,
		target:          opts.Target,
		polarisConsumer: pbb.polarisConsumer,
		subConns:        make(map[balancer.SubConn]model.InstanceKey),
		subConnStates:   make(map[balancer.SubConn]connectivity.State),
		syncInstancesCh: make(chan struct{}, 1),
		close:           make(chan struct{}),
	}
}

// Name 返回Name
func (pbb *polarisBalancerBuilder) Name() string {
	return Name
}

type attributeKey int

const (
	attributeKeyTargetNamespace attributeKey = 1
	attributeKeyTargetService   attributeKey = 2
	attributeKeyPolarisConsumer attributeKey = 3
	attributeKeySyncInterval    attributeKey = 4
	attributeKeyMetadata        attributeKey = 5
	attributeKeySourceService   attributeKey = 6
	attributeKeyHeaderPrefix    attributeKey = 7
)

// polarisBalancer
type polarisBalancer struct {
	cc              balancer.ClientConn
	target          resolver.Target
	polarisConsumer api.ConsumerAPI
	syncInterval    time.Duration
	metadata        map[string]string
	sourceService   *model.ServiceInfo
	headerPrefix    []string
	namespace       string
	service         string
	// subConns
	// addrs 从polaris获取的实例信息, 不会并发读写 key:instancesKey value:addr
	addrs map[model.InstanceKey]string
	// mu 保护subConn  subConnStates
	mu            sync.Mutex
	subConns      map[balancer.SubConn]model.InstanceKey
	subConnStates map[balancer.SubConn]connectivity.State
	// syncInstancesCh 立即刷新一次instances
	syncInstancesCh chan struct{}
	close           chan struct{}
	closeOnce       sync.Once
}

// HandleResolvedAddrs 实现旧版的 balancer.Balancer, 理论上不会被调用
func (b *polarisBalancer) HandleResolvedAddrs(addrs []resolver.Address, err error) {
	_ = b.UpdateClientConnState(balancer.ClientConnState{
		ResolverState: resolver.State{
			Addresses: addrs,
		},
	})
}

// HandleResolvedAddrs 实现新版的 balancer.V2Balancer
// resolver 调用 resolver.ClientConn.UpdateState 传递 addrs 时会调用对应balancer的这个函数
func (b *polarisBalancer) UpdateClientConnState(cs balancer.ClientConnState) error {
	grpclog.Infof("polarisPickerBuilder: HandleResolvedAddrs: cs:%v", cs)
	attr := cs.ResolverState.Attributes
	if namespace, ok := attr.Value(attributeKeyTargetNamespace).(string); ok {
		b.namespace = namespace
	}

	if service, ok := attr.Value(attributeKeyTargetService).(string); ok {
		b.service = service
	}

	if polarisConsumer, ok := attr.Value(attributeKeyPolarisConsumer).(api.ConsumerAPI); ok {
		b.polarisConsumer = polarisConsumer
	}

	if syncInterval, ok := attr.Value(attributeKeySyncInterval).(time.Duration); ok {
		b.syncInterval = syncInterval
	}
	if b.syncInterval <= 0 {
		const defaultSyncInterval = time.Second * 5
		b.syncInterval = defaultSyncInterval
	}

	if meta, ok := attr.Value(attributeKeyMetadata).(map[string]string); ok {
		b.metadata = meta
	} else {
		b.metadata = nil
	}

	if sourceService, ok := attr.Value(attributeKeySourceService).(*model.ServiceInfo); ok {
		b.sourceService = sourceService
	} else {
		b.sourceService = nil
	}

	if headerPrefix, ok := attr.Value(attributeKeyHeaderPrefix).([]string); ok {
		b.headerPrefix = headerPrefix
	} else {
		b.headerPrefix = nil
	}

	go b.daemon()
	return nil
}

// daemon updatePolarisInstances
func (b *polarisBalancer) daemon() {
	t := time.NewTicker(b.syncInterval)
	defer t.Stop()
	for {
		b.updatePolarisInstances()
		select {
		case <-b.close:
			return
		case <-b.syncInstancesCh:
		case <-t.C:
		}
	}
}

// updatePolarisInstances 更新实例信息并diff, 新的建立连接, 删除的断开连接
// TODO polaris SDK能否提供watch机制
func (b *polarisBalancer) updatePolarisInstances() {
	defer func() {
		if rec := recover(); rec != nil {
			grpclog.Errorf("polarisBalancer: updatePolarisInstances panic:%v", rec)
		}
	}()
	var getInstancesReq *api.GetInstancesRequest
	getInstancesReq = &api.GetInstancesRequest{}
	getInstancesReq.Namespace = b.namespace
	getInstancesReq.Service = b.service
	if b.metadata != nil {
		getInstancesReq.Metadata = b.metadata
	}
	if b.sourceService != nil {
		getInstancesReq.SourceService = b.sourceService
	}
	resp, err := b.polarisConsumer.GetInstances(getInstancesReq)
	if err != nil {
		grpclog.Errorf("polarisBalancer: failed to get instances, err:%s", err)
		return
	}
	// TODO 限制建立连接的instance数量
	newAddrs := make(map[model.InstanceKey]string)
	for _, instance := range resp.Instances {
		addr := fmt.Sprintf("%s:%d", instance.GetHost(), instance.GetPort())
		instanceKey := instance.GetInstanceKey()
		newAddrs[instanceKey] = addr
	}
	if reflect.DeepEqual(newAddrs, b.addrs) {
		return
	}
	b.processSubConn(newAddrs)
	b.addrs = newAddrs
}

// processSubConn 根据最新地址列表处理subConn的创建和关闭
func (b *polarisBalancer) processSubConn(newAddrs map[model.InstanceKey]string) {
	grpclog.Infof("polarisBalancer: got %d new instances:%v", len(newAddrs), newAddrs)
	// 在新的不在老的: 新建连接
	for instanceKey, addr := range newAddrs {
		if _, ok := b.addrs[instanceKey]; !ok {
			grpclog.Infof("polarisBalancer: NewSubConn for instance:%s", instanceKey)
			sc, err := b.cc.NewSubConn([]resolver.Address{{Addr: addr}}, balancer.NewSubConnOptions{HealthCheckEnabled: false})
			if err != nil {
				grpclog.Errorf("polarisBalancer: failed to NewSubConn, err:%s", err)
				// TODO 报告北极星实例信息有误或连不上实例?
				continue
			}
			b.mu.Lock()
			b.subConns[sc] = instanceKey
			if _, ok := b.subConnStates[sc]; !ok {
				b.subConnStates[sc] = connectivity.Idle
			}
			b.mu.Unlock()
			sc.Connect() // 内部是异步建立连接的
		}
	}
	// 在旧的不在新的: 关闭连接
	b.mu.Lock()
	for sc, instanceKey := range b.subConns {
		if _, ok := newAddrs[instanceKey]; !ok {
			grpclog.Infof("polarisBalancer: RemoveSubConn for instance:%s", instanceKey)
			delete(b.subConns, sc)
			b.cc.RemoveSubConn(sc)
		}
	}
	b.mu.Unlock()
}

// UpdateSubConnState 实现新版的 balancer.V2Balancer
// resolver返回error时会回调 Balancer 的此函数
func (b *polarisBalancer) ResolverError(err error) {
	// polaris resolver返回的总是为nil
}

// HandleResolvedAddrs 实现旧版的 balancer.Balancer, 理论上不会被调用
func (b *polarisBalancer) HandleSubConnStateChange(sc balancer.SubConn, s connectivity.State) {
	b.UpdateSubConnState(sc, balancer.SubConnState{ConnectivityState: s})
}

// UpdateSubConnState 实现新版的 balancer.V2Balancer
// grpc.ClientConn 的subConn连接状态变更时会回调 Balancer 的此函数
func (b *polarisBalancer) UpdateSubConnState(sc balancer.SubConn, s balancer.SubConnState) {
	grpclog.Infof("polarisBalancer: HandleSubConnStateChange: %p, %v", sc, s)
	b.mu.Lock()
	b.subConnStates[sc] = s.ConnectivityState
	b.mu.Unlock()
	switch s.ConnectivityState {
	case connectivity.Idle:
		sc.Connect()
	case connectivity.Shutdown:
		b.mu.Lock()
		delete(b.subConnStates, sc)
		b.mu.Unlock()
		// regenerate picker
	case connectivity.Ready:
		// regenerate picker
	case connectivity.Connecting:
		// do nothing
		return
	case connectivity.TransientFailure:
		if s.ConnectionError != nil {
			// TODO 报告北极星连接不上此实例?
			b.mu.Lock()
			instanceKey := b.subConns[sc]
			b.mu.Unlock()
			grpclog.Warningf("polarisBalancer: ConnectionError err:%s, instanceKey:%s",
				instanceKey, s.ConnectionError)
			return
		}
	}
	picker := &polarisPicker{
		namespace:            b.namespace,
		service:              b.service,
		metadata:             b.metadata,
		sourceService:        b.sourceService,
		headerPrefix:         b.headerPrefix,
		polarisConsumer:      b.polarisConsumer,
		subConn:              b.getReadySubConn(),
		syncPolarisInstances: b.syncPolarisInstances,
	}
	state := b.aggregateSubConnStates()
	if state == connectivity.TransientFailure {
		picker.err = balancer.ErrNoSubConnAvailable
	}
	grpclog.Infof("polarisBalancer: UpdateState: state:%+v, picker.err:%v", sc, picker.err)
	b.cc.UpdateState(balancer.State{ConnectivityState: state, Picker: picker})
}

func (b *polarisBalancer) syncPolarisInstances() {
	select {
	case b.syncInstancesCh <- struct{}{}:
	default:
	}
}

// Close 关闭Balancer
func (b *polarisBalancer) Close() {
	// 后续polarisConsumer是balancer创建的而不是传入的, 则在此关闭
	b.closeOnce.Do(func() {
		close(b.close)
	})
}

// getReadySubConn 获取所有Ready状态的SubConn
func (b *polarisBalancer) getReadySubConn() map[model.InstanceKey]balancer.SubConn {
	b.mu.Lock()
	defer b.mu.Unlock()
	readySubConn := make(map[model.InstanceKey]balancer.SubConn, len(b.subConns))
	for k, v := range b.subConns {
		if b.subConnStates[k] == connectivity.Ready {
			readySubConn[v] = k
		}
	}
	return readySubConn
}

// aggregateSubConnStates
// 只要有一个ready, 那就是ready
func (b *polarisBalancer) aggregateSubConnStates() connectivity.State {
	b.mu.Lock()
	defer b.mu.Unlock()

	var numConnecting uint64

	for sc := range b.subConns {
		if state, ok := b.subConnStates[sc]; ok {
			switch state {
			case connectivity.Ready:
				return connectivity.Ready
			case connectivity.Connecting:
				numConnecting++
			}
		}
	}
	if numConnecting > 0 {
		return connectivity.Connecting
	}
	return connectivity.TransientFailure
}

// polarisPicker implement picker
type polarisPicker struct {
	err             error
	namespace       string
	service         string
	metadata        map[string]string
	sourceService   *model.ServiceInfo
	headerPrefix    []string
	polarisConsumer api.ConsumerAPI
	// subConns readOnly map
	subConn map[model.InstanceKey]balancer.SubConn
	//
	syncPolarisInstances func()
}

// Pick 选择一个SubConn, 是核心方法.
func (pp *polarisPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	if pp.err != nil {
		grpclog.Infof("polarisPicker: ret err:%s", pp.err)
		return balancer.PickResult{}, pp.err
	}
	if len(pp.subConn) == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	var getOneInstancesReq *api.GetOneInstanceRequest
	getOneInstancesReq = &api.GetOneInstanceRequest{}
	getOneInstancesReq.Namespace = pp.namespace
	getOneInstancesReq.Service = pp.service
	if pp.metadata != nil {
		getOneInstancesReq.Metadata = pp.metadata
	}
	if pp.sourceService != nil {
		// 如果在Conf中配置了SourceService，则优先使用配置
		getOneInstancesReq.SourceService = pp.sourceService
	} else {
		// 如果没有配置，则使用gRPC Header作为规则路由元数据
		if md, ok := metadata.FromOutgoingContext(info.Ctx); ok {
			mp := make(map[string]string)
			for kvs := range md {
				if pp.headerPrefix != nil {
					// 如果配置的前缀不为空，则要求header匹配前缀
					for _, prefix := range pp.headerPrefix {
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
				getOneInstancesReq.SourceService = &model.ServiceInfo{
					Metadata: mp,
				}
			}
		}
	}
	resp, err := pp.polarisConsumer.GetOneInstance(getOneInstancesReq)
	if err != nil || len(resp.Instances) < 1 {
		grpclog.Errorf("polarisBalancer: polarisConsumer.GetOneInstance err:%v", err)
		// polarisConsumer返回了错误, 此时返回没有可用连接错误.
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	pickedInstance := resp.Instances[0]
	sc, ok := pp.subConn[pickedInstance.GetInstanceKey()]
	if !ok {
		grpclog.Errorf("polarisBalancer: polarisConsumer GetOneInstance not ready, instanceKey:%s",
			pickedInstance.GetInstanceKey())
		// 说明GetOneInstance拿到的实例在Balancer记录状态不ready
		// 触发balancer更新一次新的实例列表
		// 然后再次查找，如果还是没有则返回没有可用连接错误
		if pp.syncPolarisInstances != nil {
			pp.syncPolarisInstances()

		}
		sc, ok = pp.subConn[pickedInstance.GetInstanceKey()]
		if !ok {
			return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
		}
	}
	return balancer.PickResult{SubConn: sc, Done: pp.makePickResultDone(pickedInstance)}, nil
}

// makePickResultDone 构造PickResult.Done
func (pp *polarisPicker) makePickResultDone(pickedInstance model.Instance) func(balancer.DoneInfo) {
	start := time.Now()
	return func(doneInfo balancer.DoneInfo) {
		duration := time.Since(start)
		callResult := &api.ServiceCallResult{}
		callResult.SetCalledInstance(pickedInstance)
		retStatus := api.RetSuccess
		if doneInfo.Err != nil {
			if rpcErr, ok := status.FromError(doneInfo.Err); ok {
				if rpcErr.Code() == codes.Unavailable ||
					rpcErr.Code() == codes.DeadlineExceeded {
					retStatus = api.RetFail
				}
			}
		}
		callResult.SetRetStatus(retStatus)
		callResult.SetRetCode(0)
		callResult.SetDelay(duration)
		err := pp.polarisConsumer.UpdateServiceCallResult(callResult)
		if err != nil {
			grpclog.Errorf("polarisBalancer: polarisConsumer.UpdateServiceCallResult err:%v", err)
			return
		}
		grpclog.Infof("polarisBalancer: polarisConsumer send callResult:%v", callResult)
	}
}

// randomGetSubConn 利用map随机的特性随机获取一个subConn
func (pp *polarisPicker) randomGetSubConn() balancer.SubConn {
	var sc balancer.SubConn
	for _, sc = range pp.subConn {
		break
	}
	return sc
}
