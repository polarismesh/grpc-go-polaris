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
	"fmt"
	"strings"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/flow/data"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/specification/source/go/api/v1/traffic_manage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
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

	quotaReq := p.buildQuotaRequest(ctx, req, info)
	if quotaReq == nil {
		return handler(ctx, req)
	}

	future, err := p.limitAPI.GetQuota(quotaReq)
	if nil != err {
		grpclog.Errorf("[Polaris][RateLimit] fail to get quota %#v: %v", quotaReq, err)
		return handler(ctx, req)
	}

	if rsp := future.Get(); rsp.Code == api.QuotaResultLimited {
		return nil, status.Error(codes.ResourceExhausted, rsp.Info)
	}
	return handler(ctx, req)
}

func (p *RateLimitInterceptor) buildQuotaRequest(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo) api.QuotaRequest {

	fullMethodName := info.FullMethod
	tokens := strings.Split(fullMethodName, "/")
	if len(tokens) != 3 {
		return nil
	}
	namespace := DefaultNamespace
	if len(p.namespace) > 0 {
		namespace = p.namespace
	}

	quotaReq := api.NewQuotaRequest()
	quotaReq.SetNamespace(namespace)
	quotaReq.SetService(extractBareServiceName(fullMethodName))
	quotaReq.SetMethod(extractBareMethodName(fullMethodName))

	if len(p.svcName) > 0 {
		quotaReq.SetService(p.svcName)
		quotaReq.SetMethod(fullMethodName)
	}

	matchs, ok := p.fetchArguments(quotaReq.(*model.QuotaRequestImpl))
	if !ok {
		return quotaReq
	}
	header, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		header = metadata.MD{}
	}

	for i := range matchs {
		item := matchs[i]
		switch item.GetType() {
		case traffic_manage.MatchArgument_CALLER_SERVICE:
			serviceValues := header.Get(polarisCallerServiceKey)
			namespaceValues := header.Get(polarisCallerNamespaceKey)
			if len(serviceValues) > 0 && len(namespaceValues) > 0 {
				quotaReq.AddArgument(model.BuildCallerServiceArgument(namespaceValues[0], serviceValues[0]))
			}
		case traffic_manage.MatchArgument_HEADER:
			values := header.Get(item.GetKey())
			if len(values) > 0 {
				quotaReq.AddArgument(model.BuildHeaderArgument(item.GetKey(), fmt.Sprintf("%+v", values[0])))
			}
		case traffic_manage.MatchArgument_CALLER_IP:
			if pr, ok := peer.FromContext(ctx); ok && pr.Addr != nil {
				address := pr.Addr.String()
				addrSlice := strings.Split(address, ":")
				if len(addrSlice) == 2 {
					clientIP := addrSlice[0]
					quotaReq.AddArgument(model.BuildCallerIPArgument(clientIP))
				}
			}
		}
	}

	return quotaReq
}

func (p *RateLimitInterceptor) fetchArguments(req *model.QuotaRequestImpl) ([]*traffic_manage.MatchArgument, bool) {
	engine := p.limitAPI.SDKContext().GetEngine()

	getRuleReq := &data.CommonRateLimitRequest{
		DstService: model.ServiceKey{
			Namespace: req.GetNamespace(),
			Service:   req.GetService(),
		},
		Trigger: model.NotifyTrigger{
			EnableDstRateLimit: true,
		},
		ControlParam: model.ControlParam{
			Timeout: time.Millisecond * 500,
		},
	}

	if err := engine.SyncGetResources(getRuleReq); err != nil {
		grpclog.Errorf("[Polaris][RateLimit] ns:%s svc:%s get RateLimit Rule fail : %+v",
			req.GetNamespace(), req.GetService(), err)
		return nil, false
	}

	svcRule := getRuleReq.RateLimitRule
	if svcRule == nil || svcRule.GetValue() == nil {
		grpclog.Warningf("[Polaris][RateLimit] ns:%s svc:%s get RateLimit Rule is nil",
			req.GetNamespace(), req.GetService())
		return nil, false
	}

	rules, ok := svcRule.GetValue().(*traffic_manage.RateLimit)
	if !ok {
		grpclog.Errorf("[Polaris][RateLimit] ns:%s svc:%s get RateLimit Rule invalid",
			req.GetNamespace(), req.GetService())
		return nil, false
	}

	ret := make([]*traffic_manage.MatchArgument, 0, 4)
	for i := range rules.GetRules() {
		rule := rules.GetRules()[i]
		if len(rule.GetArguments()) == 0 {
			continue
		}

		ret = append(ret, rule.Arguments...)
	}
	return ret, true
}
