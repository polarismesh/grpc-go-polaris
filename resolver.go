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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

type ResolverContext struct {
	Target        resolver.Target
	Host          string
	Port          int
	SourceService model.ServiceInfo
}

// ResolverInterceptor 从 polaris sdk 拉取完 instances 执行过滤
type ResolverInterceptor interface {
	After(ctx *ResolverContext, response *model.InstancesResponse) *model.InstancesResponse
}

var resolverInterceptors []ResolverInterceptor

// RegisterResolverInterceptor
// NOTE: this function must only be called during initialization time (i.e. in
// an init() function), and is not thread-safe. If multiple RegisterInterceptor are
// registered with the same name, the one registered last will take effect.
func RegisterResolverInterceptor(i ResolverInterceptor) {
	resolverInterceptors = append(resolverInterceptors, i)
}

func NewResolver(ctx api.SDKContext) *resolverBuilder {
	return &resolverBuilder{
		sdkCtx: ctx,
	}
}

type resolverBuilder struct {
	sdkCtx api.SDKContext
}

// Scheme polaris scheme
func (rb *resolverBuilder) Scheme() string {
	return scheme
}

func targetToOptions(target resolver.Target) (*dialOptions, error) {
	options := &dialOptions{}
	if len(target.URL.RawQuery) > 0 {
		var optionsStr string
		values := target.URL.Query()
		if len(values) > 0 {
			optionValues := values[optionsKey]
			if len(optionValues) > 0 {
				optionsStr = optionValues[0]
			}
		}
		if len(optionsStr) > 0 {
			value, err := base64.URLEncoding.DecodeString(optionsStr)
			if nil != err {
				GetLogger().Error("[Polaris][Resolver] fail to decode endpoint %s, options %s: %w", target.URL.Opaque, optionsStr, err)
				return nil, fmt.Errorf("fail to decode endpoint %s, options %s: %w", target.URL.Opaque, optionsStr, err)
			}
			if err = json.Unmarshal(value, options); nil != err {
				return nil, fmt.Errorf("fail to unmarshal options %s: %w", string(value), err)
			}
		}
	}
	return options, nil
}

// Build Implement the Build method in the Resolver Builder interface,
// build a new Resolver resolution service address for the specified Target,
// and pass the polaris information to the balancer through attr
func (rb *resolverBuilder) Build(
	target resolver.Target,
	cc resolver.ClientConn,
	opts resolver.BuildOptions) (resolver.Resolver, error) {
	options, err := targetToOptions(target)
	if nil != err {
		return nil, err
	}
	host, port, err := parseHost(target.URL.Host)
	if err != nil {
		return nil, err
	}

	if rb.sdkCtx == nil {
		sdkCtx, err := PolarisContext()
		if nil != err {
			return nil, err
		}
		rb.sdkCtx = sdkCtx
	}
	options.SDKContext = rb.sdkCtx

	ctx, cancel := context.WithCancel(context.Background())
	d := &polarisNamingResolver{
		ctx:      ctx,
		cancel:   cancel,
		cc:       cc,
		target:   target,
		options:  options,
		host:     host,
		port:     port,
		consumer: api.NewConsumerAPIByContext(rb.sdkCtx),
		eventCh:  make(chan struct{}, 1),
	}
	go d.watcher()
	d.ResolveNow(resolver.ResolveNowOptions{})
	return d, nil
}

func parseHost(target string) (string, int, error) {
	splits := strings.Split(target, ":")
	if len(splits) > 2 {
		return "", 0, errors.New("error format host")
	}
	if len(splits) == 1 {
		return target, 0, nil
	}
	port, err := strconv.Atoi(splits[1])
	if err != nil {
		return "", 0, err
	}
	return splits[0], port, nil
}

type polarisNamingResolver struct {
	ctx    context.Context
	cancel context.CancelFunc
	cc     resolver.ClientConn
	// rn channel is used by ResolveNow() to force an immediate resolution of the target.
	eventCh  chan struct{}
	options  *dialOptions
	target   resolver.Target
	host     string
	port     int
	consumer api.ConsumerAPI
}

// ResolveNow The method is called by the gRPC framework to resolve the target name
func (pr *polarisNamingResolver) ResolveNow(opt resolver.ResolveNowOptions) { // 立即resolve，重新查询服务信息
	select {
	case pr.eventCh <- struct{}{}:
	default:
	}
}

func getNamespace(options *dialOptions) string {
	namespace := DefaultNamespace
	if len(options.Namespace) > 0 {
		namespace = options.Namespace
	}
	return namespace
}

const keyDialOptions = "options"

const keyResponse = "response"

func (pr *polarisNamingResolver) lookup() (*resolver.State, error) {
	instancesRequest := &api.GetInstancesRequest{
		GetInstancesRequest: model.GetInstancesRequest{
			Namespace:       getNamespace(pr.options),
			Service:         pr.host,
			SkipRouteFilter: false,
		},
	}
	if len(pr.options.DstMetadata) > 0 {
		instancesRequest.Metadata = pr.options.DstMetadata
	}

	resp, err := pr.consumer.GetInstances(instancesRequest)
	if nil != err {
		return nil, err
	}
	rc := &ResolverContext{
		Target: pr.target,
		Host:   pr.host,
		Port:   pr.port,
		SourceService: model.ServiceInfo{
			Namespace: getNamespace(pr.options),
			Service:   pr.host,
			Metadata:  pr.options.DstMetadata,
		},
	}
	for _, interceptor := range resolverInterceptors {
		resp = interceptor.After(rc, resp)
	}

	state := &resolver.State{
		Attributes: attributes.New(keyDialOptions, pr.options).WithValue(keyResponse, resp),
	}
	for _, instance := range resp.Instances {
		state.Addresses = append(state.Addresses, resolver.Address{
			Addr: fmt.Sprintf("%s:%d", instance.GetHost(), instance.GetPort()),
		})
	}
	return state, nil
}

func (pr *polarisNamingResolver) doWatch() (*model.WatchAllInstancesResponse, error) {
	watchRequest := &api.WatchAllInstancesRequest{
		WatchAllInstancesRequest: model.WatchAllInstancesRequest{
			InstancesListener: pr,
			WatchMode:         model.WatchModeNotify,
			ServiceKey: model.ServiceKey{
				Namespace: getNamespace(pr.options),
				Service:   pr.host,
			},
		},
	}
	return pr.consumer.WatchAllInstances(watchRequest)
}

func (pr *polarisNamingResolver) watcher() {
	watchRsp, err := pr.doWatch()
	if nil != err {
		GetLogger().Error("[Polaris][Resolver] fail to do watch for namespace=%s service=%s: %v",
			pr.options.Namespace, pr.host, err)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer func() {
		ticker.Stop()
		watchRsp.CancelWatch()
		close(pr.eventCh)
	}()
	for {
		select {
		case <-pr.ctx.Done():
			GetLogger().Info("[Polaris][Resolver] exit watch instance change event for namespace=%s service=%s: %v",
				pr.options.Namespace, pr.host)
			return
		case <-pr.eventCh:
			pr.doRefresh()
		case <-ticker.C:
			pr.doRefresh()
		}
	}
}

func (pr *polarisNamingResolver) doRefresh() {
	var (
		state *resolver.State
		err   error
	)
	state, err = pr.lookup()
	if err != nil {
		GetLogger().Error("[Polaris][Resolver] fail to do lookup %s: %v", pr.target.URL.Host, err)
		pr.cc.ReportError(err)
		return
	}
	if err = pr.cc.UpdateState(*state); nil != err {
		GetLogger().Error("[Polaris][Resolver] fail to do update state %s: %v", pr.target.URL.Host, err)
	}
}

func (pr *polarisNamingResolver) OnInstancesUpdate(rsp *model.InstancesResponse) {
	defer func() {
		// cover 住 chan 的写入一个 closed 导致的 panic
		_ = recover()
	}()
	pr.eventCh <- struct{}{}
}

// Close resolver closed
func (pr *polarisNamingResolver) Close() {
	pr.cancel()
}
