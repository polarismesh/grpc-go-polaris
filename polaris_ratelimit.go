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
	"strings"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
)

// 服务端限流器接口
type Limiter interface {
	// Limit If the current is limited, return true and an error message
	Limit() (bool, error)
}

// Polaris限流器
type PolarisLimiter struct {
	Namespace string
	Service   string
	Labels    map[string]string
	LimitAPI  api.LimitAPI
}

// 限流方法
func (pl *PolarisLimiter) Limit() (bool, error) {
	quotaReq := api.NewQuotaRequest()
	quotaReq.SetNamespace(pl.Namespace)
	quotaReq.SetService(pl.Service)
	quotaReq.SetLabels(pl.Labels)
	//调用配额获取接口
	future, err := pl.LimitAPI.GetQuota(quotaReq)
	if nil != err {
		return false, err
	}
	resp := future.Get()

	if strings.Compare("", resp.Info) != 0 {
		grpclog.Infof("get quota response info : %s", resp.Info)
	}

	if api.QuotaResultOk == resp.Code {
		return false, nil
	} else {
		return true, nil
	}
}

// Unary 限流拦截器
func UnaryServerInterceptor(limiter Limiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		rejected, err := limiter.Limit()
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if rejected {
			return nil, status.Errorf(codes.ResourceExhausted,
				"%s is rejected by polaris rate limiter, please retry later.", info.FullMethod)
		}
		return handler(ctx, req)
	}
}

// Stream 限流拦截器
func StreamServerInterceptor(limiter Limiter) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream,
		info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		rejected, err := limiter.Limit()
		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
		if rejected {
			return status.Errorf(codes.ResourceExhausted,
				"%s is rejected by polaris rate limiter, please retry later.", info.FullMethod)
		}
		return handler(srv, stream)
	}
}
