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
	"github.com/polarismesh/polaris-go/pkg/config"
	"google.golang.org/grpc"
	"strings"
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
	Namespace       string            `json:"Namespace"`
	DstMetadata     map[string]string `json:"dst_metadata"`
	SrcMetadata     map[string]string `json:"src_metadata"`
	SrcService      string            `json:"src_service"`
	// 可选，规则路由Meta匹配前缀，用于过滤作为路由规则的gRPC Header
	HeaderPrefix []string                  `json:"header_prefix"`
	Config       *config.ConfigurationImpl `json:"-"`
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
func WithHeaderPrefix(headerPrefix []string) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.HeaderPrefix = headerPrefix
	})
}

// WithSrcService set the srcMetadata for dial service routing
func WithConfig(config *config.ConfigurationImpl) DialOption {
	return newFuncDialOption(func(options *dialOptions) {
		options.Config = config
	})
}

const (
	scheme     = "polaris"
	prefix     = scheme + "://"
	optionsKey = "options"
)

// DialContext dial target and get connection
func DialContext(ctx context.Context, target string, opts ...DialOption) (conn *grpc.ClientConn, err error) {
	options := &dialOptions{}
	for _, opt := range opts {
		opt.apply(options)
	}
	if !strings.HasPrefix(target, prefix) {
		// not polaris target, go through gRPC resolver
		return grpc.DialContext(ctx, target, options.gRPCDialOptions...)
	}
	options.gRPCDialOptions = append(options.gRPCDialOptions, grpc.WithDefaultServiceConfig(LoadBalanceConfig))
	jsonStr, err := json.Marshal(options)
	if nil != err {
		return nil, fmt.Errorf("fail to marshal options: %w", err)
	}
	endpoint := base64.URLEncoding.EncodeToString(jsonStr)
	target = fmt.Sprintf("%s?%s=%s", target, optionsKey, endpoint)
	return grpc.DialContext(ctx, target, options.gRPCDialOptions...)
}

// BuildTarget build the invoker grpc target
func BuildTarget(target string, opts ...DialOption) (string, error) {
	options := &dialOptions{}
	for _, opt := range opts {
		opt.apply(options)
	}
	jsonStr, err := json.Marshal(options)
	if nil != err {
		return "", fmt.Errorf("fail to marshal options: %w", err)
	}
	SetPolarisConfig(options.Config)
	endpoint := base64.URLEncoding.EncodeToString(jsonStr)
	target = fmt.Sprintf(prefix+"%s?%s=%s", target, optionsKey, endpoint)
	return target, nil
}
