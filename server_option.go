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
	gRPCServerOptions []grpc.ServerOption
	SDKContext        api.SDKContext
	namespace         string
	svcName           string
	ttl               int
	metadata          map[string]string
	host              string
	port              int
	version           string
	token             string
	ctrlOptions
}

type ctrlOptions struct {
	delayRegisterEnable         *bool
	delayRegisterStrategy       DelayStrategy
	gracefulStopEnable          *bool
	gracefulStopMaxWaitDuration time.Duration
}

func (s *serverOptions) setDefault() {
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

func WithSDKContext(sdkContext api.SDKContext) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.SDKContext = sdkContext
	})
}

// WithServiceName set the application to register instance
func WithServiceName(svcName string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.svcName = svcName
	})
}

// WithHeartbeatEnable enables the heartbeat task to instance
// Deprecated: will remove in 1.4
func WithHeartbeatEnable(enable bool) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {

	})
}

func setDelayRegisterEnable(options *serverOptions, enable bool) {
	options.delayRegisterEnable = &enable
}

func setDelayRegisterStrategy(options *serverOptions, strategy DelayStrategy) {
	options.delayRegisterStrategy = strategy
}

// WithDelayRegisterEnable enables delay register
func WithDelayRegisterEnable(strategy DelayStrategy) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setDelayRegisterEnable(options, true)
		setDelayRegisterStrategy(options, strategy)
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

func setGracefulStopEnable(options *serverOptions, enable bool) {
	options.gracefulStopEnable = &enable
}

func setGracefulStopMaxWaitDuration(options *serverOptions, duration time.Duration) {
	options.gracefulStopMaxWaitDuration = duration
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
