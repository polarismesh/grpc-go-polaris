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
	"time"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc/grpclog"
)

// 服务端注册器接口
type Register interface {
	RegisterAndHeartbeat()
	DeRegister()
}

// Polaris限流器
type PolarisRegister struct {
	Namespace            string
	Service              string
	ServiceToken         string
	Host                 string
	Port                 int
	HeartbeatIntervalSec time.Duration
	ProviderAPI          api.ProviderAPI
}

// 服务注册和心跳上报
func (pr *PolarisRegister) RegisterAndHeartbeat() {
	//注册服务
	request := &api.InstanceRegisterRequest{}
	request.Namespace = pr.Namespace
	request.Service = pr.Service
	request.ServiceToken = pr.ServiceToken
	request.Host = pr.Host
	request.Port = pr.Port
	request.SetTTL(2)
	_, err := pr.ProviderAPI.Register(request)
	if nil != err {
		grpclog.Fatalf("fail to register instance, err %v", err)
	}

	// 心跳上报
	ticker := time.NewTicker(pr.HeartbeatIntervalSec)
	for range ticker.C {
		hbRequest := &api.InstanceHeartbeatRequest{}
		hbRequest.Namespace = pr.Namespace
		hbRequest.Service = pr.Service
		hbRequest.Host = pr.Host
		hbRequest.Port = pr.Port
		hbRequest.ServiceToken = pr.ServiceToken
		if err = pr.ProviderAPI.Heartbeat(hbRequest); nil != err {
			grpclog.Fatalf("fail to heartbeat, error is %v", err)
		}
	}
}

// 服务反注册
func (pr *PolarisRegister) DeRegister() {
	//反注册服务
	request := &api.InstanceDeRegisterRequest{}
	request.Namespace = pr.Namespace
	request.Service = pr.Service
	request.ServiceToken = pr.ServiceToken
	request.Host = pr.Host
	request.Port = pr.Port
	err := pr.ProviderAPI.Deregister(request)
	if nil != err {
		grpclog.Fatalf("fail to deregister instance, err %v", err)
	}
}
