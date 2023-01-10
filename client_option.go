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
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
	"google.golang.org/grpc"

	"github.com/polarismesh/polaris-go/pkg/config"
	"google.golang.org/grpc"
)

// DialOption dialOptions for gRPC-Go-Polaris
type DialOption interface {
	apply(o *dialOptions)
}

// funcDialOption wraps a function that modifies dialOptions into an
// implementation of the DialOption interface.
type funcDialOption struct {
	f func(*dialOptions)
}

func (fdo *funcDialOption) apply(do *dialOptions) {
	fdo.f(do)
}

func newFuncDialOption(f func(*dialOptions)) *funcDialOption {
	return &funcDialOption{
		f: f,
	}
}

type dialOptions struct {
	gRPCDialOptions []grpc.DialOption
	Namespace       string               `json:"Namespace"`
	DstMetadata     map[string]string    `json:"dst_metadata"`
	SrcMetadata     map[string]string    `json:"src_metadata"`
	SrcService      string               `json:"src_service"`
	Config          config.Configuration `json:"-"`
	SDKContext      api.SDKContext       `json:"-"`
	Route           bool                 `json:"route"`
	CircuitBreaker  bool                 `json:"circuit_breaker"`
}

func newDialOptions() *dialOptions {
	return &dialOptions{
		gRPCDialOptions: make([]grpc.DialOption, 0, 4),
		Route:           true,
		CircuitBreaker:  true,
	}
}

// WithGRPCDialOptions set the raw gRPC dialOption
func WithGRPCDialOptions(opts ...grpc.DialOption) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.gRPCDialOptions = opts
	})
}

// WithClientNamespace set the namespace for dial service
func WithClientNamespace(namespace string) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.Namespace = namespace
	})
}

// WithDstMetadata set the dstMetadata for dial service routing
func WithDstMetadata(dstMetadata map[string]string) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.DstMetadata = dstMetadata
	})
}

// WithSrcMetadata set the srcMetadata for dial service routing
// Deprecated: will remove in 1.4
func WithSrcMetadata(srcMetadata map[string]string) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.SrcMetadata = srcMetadata
	})
}

// WithSrcService set the srcMetadata for dial service routing
func WithSrcService(srcService string) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.SrcService = srcService
	})
}

// WithHeaderPrefix set the header filter to get the header values to routing
// Deprecated: will remove in 1.4
func WithHeaderPrefix(headerPrefix []string) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
	})
}

// WithPolarisConfig set polaris configuration
func WithPolarisConfig(config config.Configuration) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.Config = polarisCfg
	})
}

// WithPolarisContext set polaris SDKContext
func WithPolarisContext(sdkContext api.SDKContext) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.SDKContext = sdkContext
	})
}

// WithDisableRouter close polaris route ability
func WithDisableRouter() DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.Route = false
	})
}

// WithEnableRouter open polaris route ability
func WithEnableRouter() DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.Route = true
	})
}

// WithDisableCircuitBreaker close polaris circuitbreaker ability
func WithDisableCircuitBreaker() DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.CircuitBreaker = false
	})
}

// WithEnableCircuitBreaker open polaris circuitbreaker ability
func WithEnableCircuitBreaker() DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.CircuitBreaker = true
	})
}
