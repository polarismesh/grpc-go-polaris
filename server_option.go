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
	"time"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"
)

type serverOptions struct {
	// gRPCServerOptions 保留用户自己的 grpc.ServerOption 列表数据
	gRPCServerOptions []grpc.ServerOption
	// sdkCtx 北极星客户端核心对象
	sdkCtx api.SDKContext
	// namespace 服务所在的命名空间
	namespace string
	// svcName 服务名称
	svcName string
	// ttl 服务实例心跳的 TTL 时间，单位秒
	ttl int
	// metadata 服务实例的元数据
	metadata map[string]string
	// host 服务实例的 IP 地址
	host string
	// port 服务实例的端口
	port int
	// version 服务实例的版本号
	version string
	// token 如果服务端开启了鉴权，则需要设置该 token
	token string
	ctrlOptions
}

type ctrlOptions struct {
	delayRegisterEnable         *bool
	delayRegisterStrategy       DelayStrategy
	gracefulStopEnable          *bool
	gracefulStopMaxWaitDuration time.Duration
	enableRatelimit             *bool
}

func (s *serverOptions) setDefault() error {
	if len(s.namespace) == 0 {
		s.namespace = DefaultNamespace
	}
	if s.ttl == 0 {
		s.ttl = DefaultTTL
	}
	if s.delayRegisterEnable == nil {
		setDelayRegisterEnable(s, false)
	}
	if *s.delayRegisterEnable {
		if s.delayRegisterStrategy == nil {
			setDelayRegisterStrategy(s, &NoopDelayStrategy{})
		}
	}
	if s.gracefulStopEnable == nil {
		setGracefulStopEnable(s, true)
	}
	if *s.gracefulStopEnable {
		if s.gracefulStopMaxWaitDuration <= 0 {
			setGracefulStopMaxWaitDuration(s, DefaultGracefulStopMaxWaitDuration)
		}
	}
	if s.sdkCtx == nil {
		sdkCtx, err := PolarisContext()
		if err != nil {
			return err
		}
		s.sdkCtx = sdkCtx
	}
	return nil
}

// DelayStrategy delay register strategy. e.g. wait some time
type DelayStrategy interface {
	// Allow delay strategy is allowed to continue to exec
	Allow() bool
}

// NoopDelayStrategy noop delay strategy
type NoopDelayStrategy struct{}

// Allow delay strategy is allowed to continue to exec
func (d *NoopDelayStrategy) Allow() bool {
	return true
}

// WaitDelayStrategy wait delay strategy
type WaitDelayStrategy struct {
	WaitTime time.Duration
}

// Allow delay strategy is allowed to continue to exec
func (d *WaitDelayStrategy) Allow() bool {
	timer := time.NewTimer(d.WaitTime)
	defer timer.Stop()
	<-timer.C
	return true
}

// A ServerOption sets options such as credentials, codec and keepalive parameters, etc.
type ServerOption interface {
	apply(*serverOptions)
}

// funcServerOption wraps a function that modifies serverOptions into an
// implementation of the ServerOption interface.
type funcServerOption struct {
	f func(*serverOptions)
}

func (fdo *funcServerOption) apply(do *serverOptions) {
	fdo.f(do)
}

func newFuncServerOption(f func(*serverOptions)) *funcServerOption {
	return &funcServerOption{
		f: f,
	}
}

// WithServerApplication set application name
// Deprecated: use WithServiceName to replace WithServerApplication
func WithServerApplication(application string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.svcName = application
	})
}

// WithHeartbeatEnable enables the heartbeat task to instance
// Deprecated: will remove in 1.4
func WithHeartbeatEnable(enable bool) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {

	})
}

// WithDelayStopEnable enables delay stop
// Deprecated: will remove in 1.4
func WithDelayStopEnable(strategy DelayStrategy) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		// DO Nothing
	})
}

// WithDelayStopDisable disable delay stop
// Deprecated: will remove in 1.4
func WithDelayStopDisable() ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		// DO Nothing
	})
}

// WithSDKContext 设置用户自定义的北极星 SDKContext
func WithSDKContext(sdkContext api.SDKContext) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setPolarisContext(sdkContext)
		options.sdkCtx = sdkContext
	})
}

// WithServiceName set the application to register instance
func WithServiceName(svcName string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.svcName = svcName
	})
}

// WithDelayRegisterEnable enables delay register
func WithDelayRegisterEnable(strategy DelayStrategy) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setDelayRegisterEnable(options, true)
		setDelayRegisterStrategy(options, strategy)
	})
}

// WithGracefulStopEnable enables graceful stop
func WithGracefulStopEnable(duration time.Duration) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setGracefulStopEnable(options, true)
		setGracefulStopMaxWaitDuration(options, duration)
	})
}

// WithGracefulStopDisable disable graceful stop
func WithGracefulStopDisable() ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setGracefulStopEnable(options, false)
	})
}

// WithGRPCServerOptions set the raw gRPC serverOptions
func WithGRPCServerOptions(opts ...grpc.ServerOption) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.gRPCServerOptions = opts
	})
}

// WithToken set the token to do server operations
func WithToken(token string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.token = token
	})
}

// WithServerNamespace set the namespace to register instance
func WithServerNamespace(namespace string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.namespace = namespace
	})
}

// WithServerMetadata set the metadata to register instance
func WithServerMetadata(metadata map[string]string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.metadata = metadata
	})
}

// WithServerHost set the host to register instance
func WithServerHost(host string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.host = host
	})
}

// WithServerVersion set the version to register instance
func WithServerVersion(version string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.version = version
	})
}

// WithTTL set the ttl to register instance
func WithTTL(ttl int) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.ttl = ttl
	})
}

// WithPort set the port to register instance
// 该方法非必需调用, 建议只在注册端口和程序实际监听端口需要不一致时才调用
func WithPort(port int) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.port = port
	})
}

// WithPolarisLimit 开启北极星服务端限流能力
func WithPolarisRateLimit() ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		enable := true
		options.enableRatelimit = &enable
	})
}

func setDelayRegisterEnable(options *serverOptions, enable bool) {
	options.delayRegisterEnable = &enable
}

func setDelayRegisterStrategy(options *serverOptions, strategy DelayStrategy) {
	options.delayRegisterStrategy = strategy
}

func setGracefulStopEnable(options *serverOptions, enable bool) {
	options.gracefulStopEnable = &enable
}

func setGracefulStopMaxWaitDuration(options *serverOptions, duration time.Duration) {
	options.gracefulStopMaxWaitDuration = duration
}
