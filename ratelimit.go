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
	"strings"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
)

// RateLimitInterceptor is a gRPC interceptor that implements rate limiting.
type RateLimitInterceptor struct {
	namespace string
	svcName   string
	limitAPI  api.LimitAPI
}

// NewRateLimitInterceptor creates a new RateLimitInterceptor.
func NewRateLimitInterceptor() *RateLimitInterceptor {
	polarisCtx, _ := PolarisContext()
	return &RateLimitInterceptor{limitAPI: api.NewLimitAPIByContext(polarisCtx)}
}

// WithNamespace sets the namespace of the service.
func (p *RateLimitInterceptor) WithNamespace(namespace string) *RateLimitInterceptor {
	p.namespace = namespace
	return p
}

// WithServiceName sets the service name.
func (p *RateLimitInterceptor) WithServiceName(svcName string) *RateLimitInterceptor {
	p.svcName = svcName
	return p
}

// UnaryInterceptor returns a unary interceptor for rate limiting.
func (p *RateLimitInterceptor) UnaryInterceptor(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	fullMethodName := info.FullMethod
	tokens := strings.Split(fullMethodName, "/")
	if len(tokens) != 3 {
		return handler(ctx, req)
	}
	namespace := DefaultNamespace
	if len(p.namespace) > 0 {
		namespace = p.namespace
	}
	serviceName := tokens[1]
	if len(p.svcName) > 0 {
		serviceName = p.svcName
	}
	quotaReq := api.NewQuotaRequest()
	quotaReq.SetNamespace(namespace)
	quotaReq.SetService(serviceName)
	future, err := p.limitAPI.GetQuota(quotaReq)
	if nil != err {
		grpclog.Errorf("fail to do ratelimit %s: %v", fullMethodName, err)
		return handler(ctx, req)
	}
	rsp := future.Get()
	if rsp.Code == api.QuotaResultLimited {
		return nil, status.Error(codes.ResourceExhausted, rsp.Info)
	}
	return handler(ctx, req)
}
