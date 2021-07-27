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

package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"

	polaris "github.com/polarismesh/grpc-go-polaris"
	hello "github.com/polarismesh/grpc-go-polaris/sample/model/grpc"
)

// go build -mod=vendor
// 命令行格式：./rpcServer <命名空间> <服务名> <token> <ip> <port> <label k1:label v1,label k2:label v2,...>
// ./rpcServer Development yourService yourToken yourIp yourPort yourLabels

// Hello Hello服务结构体
type Hello struct{}

// SayHello Hello服务测试方法
func (h *Hello) SayHello(ctx context.Context, req *hello.HelloRequest) (*hello.HelloResponse, error) {
	return &hello.HelloResponse{Message: fmt.Sprintf("hello %s", req.Name)}, nil
}

func main() {
	namespace, service, token, ip, port, count, labels := processArgs()

	//创建并设置 Polaris 配置对象
	configuration := api.NewConfiguration()
	//设置北极星server的地址
	configuration.GetGlobal().GetServerConnector().SetAddresses([]string{"127.0.0.1:8090"})
	//设置连接北极星server的超时时间
	configuration.GetGlobal().GetServerConnector().SetConnectTimeout(2 * time.Second)

	//使用配置获取 Polaris SDK 对象
	//Polaris Provider API
	provider, err := api.NewProviderAPI()
	if nil != err {
		log.Fatalf("fail to create provider api by default config file, err %v", err)
	}
	defer provider.Destroy()
	//Polaris Limit API
	limit, err := api.NewLimitAPI()
	if nil != err {
		log.Fatalf("fail to create limit api by default config file, err %v", err)
	}
	defer limit.Destroy()

	//构建 Polaris Unary/Stream 限流器
	limiter := &polaris.PolarisLimiter{
		Namespace: namespace,
		Service:   service,
		Labels:    labels,
		LimitAPI:  limit,
	}

	//监听端口
	address := "0.0.0.0:9090"
	listen, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	//初始化服务
	var opts []grpc.ServerOption
	opts = append(opts,
		grpc.UnaryInterceptor(polaris.UnaryServerInterceptor(limiter)),
		grpc.StreamInterceptor(polaris.StreamServerInterceptor(limiter)))
	s := grpc.NewServer(opts...)
	hello.RegisterHelloServer(s, &Hello{})
	log.Println("Listen on " + address)

	//注册服务到 Polaris
	register := &polaris.PolarisRegister{
		Namespace:    namespace,
		Service:      service,
		ServiceToken: token,
		Host:         ip,
		Port:         port,
		Count:        count,
		ProviderAPI:  provider,
	}
	go register.RegisterAndHeartbeat()

	if err := s.Serve(listen); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

//解析启动参数
func processArgs() (string, string, string, string, int, int, map[string]string) {
	// 解析启动参数
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) < 6 {
		log.Fatalf("using %s <namespace> <service> <service-token> "+
			"<ip> <port> <label k1:label v1,label k2:label v2,...>", os.Args[0])
	}
	var err error
	namespace := argsWithoutProg[0]
	service := argsWithoutProg[1]
	token := argsWithoutProg[2]
	ip := argsWithoutProg[3]
	port, err := strconv.Atoi(argsWithoutProg[4])
	if nil != err {
		log.Fatalf("fail to convert port %s to int, err %v", argsWithoutProg[4], err)
	}
	count, err := strconv.Atoi(argsWithoutProg[5])
	if nil != err {
		log.Fatalf("fail to convert count %s to int, err %v", argsWithoutProg[5], err)
	}
	labels, err := parseLabels(argsWithoutProg[6])
	if nil != err {
		log.Fatalf("fail to parse label string %s, err %v", argsWithoutProg[6], err)
	}

	return namespace, service, token, ip, port, count, labels
}

//解析标签列表
func parseLabels(labelsStr string) (map[string]string, error) {
	strLabels := strings.Split(labelsStr, ",")
	labels := make(map[string]string, len(strLabels))
	for _, strLabel := range strLabels {
		if len(strLabel) == 0 {
			continue
		}
		labelKv := strings.Split(strLabel, ":")
		if len(labelKv) != 2 {
			return nil, fmt.Errorf("invalid kv pair str %s", strLabel)
		}
		labels[labelKv[0]] = labelKv[1]
	}
	return labels, nil
}
