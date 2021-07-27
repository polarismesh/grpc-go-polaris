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
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"

	"github.com/polarismesh/polaris-go/api"
)

var (
	errAddrMisMatch = errors.New("polaris resolver: invalid uri")
)

// Conf 配置结构体
type Conf struct {
	// 北极星consumer实例
	PolarisConsumer api.ConsumerAPI
	// polaris_balancer 拉取实例信息的时间间隔，不传默认为5秒
	SyncInterval time.Duration
	// 可选，元数据信息，用于服务路由
	Metadata map[string]string
}

// NewBuilder 创建一个resolver.Builder的实例
func NewBuilder(c Conf) resolver.Builder {
	return &PolarisBuilder{
		polarisConsumer: c.PolarisConsumer,
		syncInterval:    c.SyncInterval,
		metadata:        c.Metadata,
	}
}

// Init resolver初始化函数
// 通过 resolver.Register 方法注册 Polaris Resolver
func Init(c Conf) {
	resolver.Register(NewBuilder(c))
}

// Polaris Resolver Builder
// 构建一个用于监听 name 解析更新的 Resolver
// grpc builder结构体
type PolarisBuilder struct {
	polarisConsumer api.ConsumerAPI
	syncInterval    time.Duration
	metadata        map[string]string
}

// PolarisBuilder 实现 Resolver Builder interface 中的 Scheme 方法
// 返回 Resolver 支持的 schema
// 获取builder名称，用来区分注册到grpc中的builder
func (pb *PolarisBuilder) Scheme() string {
	return "polaris"
}

// PolarisBuilder 实现 Resolver Builder interface 中的 Build 方法
// 为指定 Target 构建新的 Resolver
// 解析服务地址，通过attr传递polaris信息给balancer
func (pb *PolarisBuilder) Build(
	target resolver.Target,
	cc resolver.ClientConn,
	opts resolver.BuildOptions) (resolver.Resolver, error) {
	//log.Printf("calling polaris build\n")
	//log.Printf("target: %v\n", target)
	namespace, service, err := parseTarget(target)
	//log.Printf("namespace(%v), service(%v)\n", namespace, service)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	pr := &polarisResolver{
		namespace:       namespace,
		service:         service,
		cc:              cc,
		ctx:             ctx,
		cancel:          cancel,
		polarisConsumer: pb.polarisConsumer,
	}
	serviceConfig := cc.ParseServiceConfig(fmt.Sprintf(`{"LoadBalancingPolicy": "%s"}`, Name))
	// 符合grpc自定义balancer思想的做法是传递 polaris注册中心 address 给 balancer, balancer里去watch polaris注册中心
	// 这里兼容之前传递polarisConsumer的方式, 直接传递 polarisConsumer 给balancer
	attr := attributes.New(
		attributeKeyTargetNamespace, namespace,
		attributeKeyTargetService, service,
		attributeKeyPolarisConsumer, pb.polarisConsumer,
		attributeKeySyncInterval, pb.syncInterval,
		attributeKeyMetadata, pb.metadata,
	)
	cc.UpdateState(resolver.State{
		Addresses: []resolver.Address{
			//{Addr: "9.87.200.63:8090"},
			//{Addr: "9.157.1.185:8090"},
		},
		ServiceConfig: serviceConfig,
		Attributes:    attr,
	})
	return pr, nil
}

// Polaris Resolver
// Resolver 监听指定 Target 的变化，包括 address 和 service config 更新
// polarisResolver resolver结构体
type polarisResolver struct {
	namespace       string
	service         string
	ctx             context.Context
	cancel          context.CancelFunc
	cc              resolver.ClientConn
	polarisConsumer api.ConsumerAPI
}

// polarisResolver 实现 Resolver interface 中的 ResolveNow 方法
// ResolveNow 方法被 gRPC 框架调用以解析 target name
func (pr *polarisResolver) ResolveNow(opt resolver.ResolveNowOptions) { //立即resolve，重新查询服务信息
}

// polarisResolver 实现 Resolver interface 中的 Close 方法
// Close 用于关闭 Resolver
func (pr *polarisResolver) Close() {
	pr.cancel()
}

var (
	// regexPolarisV1 v1版的target形式: polaris://Production/grpc.service
	// 服务名限制 只允许数字、英文字母、.、-、:、_，限制128个字符
	regexPolarisV1 = regexp.MustCompile("^(Development|Production|Pre-release|Test)/([a-zA-Z0-9_:.-]{1,128})$")
	// regexPolarisNamespace
	regexPolarisNamespace = regexp.MustCompile(`^(Development|Production|Pre-release|Test)$`)
	regexPolarisService   = regexp.MustCompile(`^([a-zA-Z0-9_:.-]{1,128})$`)
)

// parseTarget 接收两种target形式
// v1版: polaris://Production/grpc.service
// v2版: polaris:///grpc.service?namespace=Production URI形式, 更具有扩展性,
// 更符合(gRPC的规范)[https://github.com/grpc/grpc/blob/master/doc/naming.md#name-syntax]
func parseTarget(target resolver.Target) (namespace, service string, err error) {
	// Authority不应该当做namespace来使用, 正确的做法是使用Endpoint URI形式传递namespace, 这里兼容一下老的
	if full := fmt.Sprintf("%s/%s", target.Authority, target.Endpoint); regexPolarisV1.MatchString(full) {
		groups := regexPolarisV1.FindStringSubmatch(full)
		namespace = groups[1]
		service = groups[2]
		return namespace, service, nil
	}
	uri, err := url.Parse(target.Endpoint)
	if err != nil {
		return "", "", fmt.Errorf("invalid target:%s, err:%w", target.Endpoint, errAddrMisMatch)
	}
	service = uri.Path
	namespace = uri.Query().Get("namespace")
	if !regexPolarisNamespace.MatchString(namespace) {
		return "", "", fmt.Errorf("invalid target:%s, err:%w", target.Endpoint, errAddrMisMatch)
	}
	if !regexPolarisService.MatchString(service) {
		return "", "", fmt.Errorf("invalid target:%s, err:%w", target.Endpoint, errAddrMisMatch)
	}
	return namespace, service, nil
}
