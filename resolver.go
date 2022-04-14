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
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/serviceconfig"

	"google.golang.org/grpc/grpclog"

	"github.com/golang/protobuf/proto"
	"github.com/polarismesh/polaris-go/pkg/model"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

type resolverBuilder struct {
}

// Scheme polaris sheme
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
				return nil, fmt.Errorf(
					"fail to decode endpoint %s, options %s: %v", target.Endpoint, optionsStr, err)
			}
			if err = json.Unmarshal(value, options); nil != err {
				return nil, fmt.Errorf("fail to unmarshal options %s: %v", string(value), err)
			}
		}
	}
	return options, nil
}

// PolarisBuilder 实现 Resolver Builder interface 中的 Build 方法
// 为指定 Target 构建新的 Resolver
// 解析服务地址，通过attr传递polaris信息给balancer
func (rb *resolverBuilder) Build(
	target resolver.Target,
	cc resolver.ClientConn,
	opts resolver.BuildOptions) (resolver.Resolver, error) {
	options, err := targetToOptions(target)
	if nil != err {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &polarisNamingResolver{
		ctx:     ctx,
		cancel:  cancel,
		cc:      cc,
		rn:      make(chan struct{}, 1),
		target:  target,
		options: options,
	}
	d.wg.Add(1)
	go d.watcher()
	d.ResolveNow(resolver.ResolveNowOptions{})
	return d, nil
}

type polarisNamingResolver struct {
	ctx    context.Context
	cancel context.CancelFunc
	cc     resolver.ClientConn
	// rn channel is used by ResolveNow() to force an immediate resolution of the target.
	rn          chan struct{}
	wg          sync.WaitGroup
	options     *dialOptions
	target      resolver.Target
	balanceOnce sync.Once
}

// ResolveNow 方法被 gRPC 框架调用以解析 target name
func (pr *polarisNamingResolver) ResolveNow(opt resolver.ResolveNowOptions) { //立即resolve，重新查询服务信息
	select {
	case pr.rn <- struct{}{}:
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

func (pr *polarisNamingResolver) lookup() (*resolver.State, api.ConsumerAPI, error) {
	sdkCtx, err := PolarisContext()
	if nil != err {
		return nil, nil, err
	}
	consumerAPI := api.NewConsumerAPIByContext(sdkCtx)
	instancesRequest := &api.GetInstancesRequest{}
	instancesRequest.Namespace = getNamespace(pr.options)
	instancesRequest.Service = pr.target.Authority
	if len(pr.options.DstMetadata) > 0 {
		instancesRequest.Metadata = pr.options.DstMetadata
	}
	sourceService := buildSourceInfo(pr.options)
	if sourceService != nil {
		// 如果在Conf中配置了SourceService，则优先使用配置
		instancesRequest.SourceService = sourceService
	}
	instancesRequest.SkipRouteFilter = true
	resp, err := consumerAPI.GetInstances(instancesRequest)
	if nil != err {
		return nil, consumerAPI, err
	}
	state := &resolver.State{}
	for _, instance := range resp.Instances {
		state.Addresses = append(state.Addresses, resolver.Address{
			Addr:       fmt.Sprintf("%s:%d", instance.GetHost(), instance.GetPort()),
			Attributes: attributes.New(keyDialOptions, pr.options),
		})
	}
	return state, consumerAPI, nil
}

func (pr *polarisNamingResolver) doWatch(
	consumerAPI api.ConsumerAPI) (model.ServiceKey, <-chan model.SubScribeEvent, error) {
	watchRequest := &api.WatchServiceRequest{}
	watchRequest.Key = model.ServiceKey{
		Namespace: getNamespace(pr.options),
		Service:   pr.target.Authority,
	}
	resp, err := consumerAPI.WatchService(watchRequest)
	if nil != err {
		return watchRequest.Key, nil, err
	}
	return watchRequest.Key, resp.EventChannel, nil
}

func (pr *polarisNamingResolver) watcher() {
	defer pr.wg.Done()
	var consumerAPI api.ConsumerAPI
	var eventChan <-chan model.SubScribeEvent
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-pr.ctx.Done():
			return
		case <-pr.rn:
		case <-eventChan:
		case <-ticker.C:
		}
		var state *resolver.State
		var err error
		state, consumerAPI, err = pr.lookup()
		if err != nil {
			pr.cc.ReportError(err)
		} else {
			pr.balanceOnce.Do(func() {
				state.ServiceConfig = &serviceconfig.ParseResult{
					Config: &grpc.ServiceConfig{
						LB: proto.String(scheme),
					},
				}
			})
			err = pr.cc.UpdateState(*state)
			if nil != err {
				grpclog.Errorf("fail to do update service %s: %v", pr.target.URL.Host, err)
			}
			var svcKey model.ServiceKey
			svcKey, eventChan, err = pr.doWatch(consumerAPI)
			if nil != err {
				grpclog.Errorf("fail to do watch for service %s: %v", svcKey, err)
			}
		}
	}
}

// Close 用于关闭 Resolver
func (pr *polarisNamingResolver) Close() {
	pr.cancel()
}
