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
	"strings"

	"google.golang.org/grpc"
)

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
	if options.Config != nil {
		setPolarisConfig(options.Config)
	}
	if !strings.HasPrefix(target, prefix) {
		// not polaris target, go through gRPC resolver
		return grpc.DialContext(ctx, target, options.gRPCDialOptions...)
	}

	lbStr := fmt.Sprintf(lbConfig, scheme)
	options.gRPCDialOptions = append(options.gRPCDialOptions, grpc.WithDefaultServiceConfig(lbStr))
	options.gRPCDialOptions = append(options.gRPCDialOptions, grpc.WithUnaryInterceptor(injectCallerInfo(options)))
	jsonStr, err := json.Marshal(options)
	if nil != err {
		return nil, fmt.Errorf("fail to marshal options: %w", err)
	}
	endpoint := base64.URLEncoding.EncodeToString(jsonStr)
	target = fmt.Sprintf("%s?%s=%s", target, optionsKey, endpoint)
	return grpc.DialContext(ctx, target, options.gRPCDialOptions...)
}

// BuildTarget build the invoker grpc target
// Deprecated: will remove in 1.4
func BuildTarget(target string, opts ...DialOption) (string, error) {
	options := &dialOptions{}
	for _, opt := range opts {
		opt.apply(options)
	}

	jsonStr, err := json.Marshal(options)
	if nil != err {
		return "", fmt.Errorf("fail to marshal options: %w", err)
	}

	if options.Config != nil {
		setPolarisConfig(options.Config)
	}
	endpoint := base64.URLEncoding.EncodeToString(jsonStr)
	target = fmt.Sprintf(prefix+"%s?%s=%s", target, optionsKey, endpoint)
	return target, nil
}

func injectCallerInfo(options *dialOptions) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = context.WithValue(ctx, polarisCallerServiceKey, options.SrcService)
		ctx = context.WithValue(ctx, polarisCallerNamespaceKey, options.Namespace)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
