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
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"

	polaris "github.com/polarismesh/grpc-go-polaris"
	hello "github.com/polarismesh/grpc-go-polaris/examples/quickstart/model/grpc"
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
	namespace, service, ip, port, heartIntervalSec := processArgs()

	// 使用配置获取 Polaris SDK 对象
	// Polaris Provider API
	provider, err := api.NewProviderAPI()
	if nil != err {
		log.Fatalf("fail to create provider api by default config file, err %v", err)
	}
	defer provider.Destroy()
	// Polaris Limit API
	limit, err := api.NewLimitAPI()
	if nil != err {
		log.Fatalf("fail to create limit api by default config file, err %v", err)
	}
	defer limit.Destroy()

	// 构建 Polaris Unary/Stream 限流器
	limiter := &polaris.PolarisLimiter{
		Namespace: namespace,
		Service:   service,
		LimitAPI:  limit,
	}

	// 监听端口
	address := "0.0.0.0:9090"
	listen, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// 初始化服务
	var opts []grpc.ServerOption
	opts = append(opts,
		grpc.UnaryInterceptor(polaris.UnaryServerInterceptor(limiter)),
		grpc.StreamInterceptor(polaris.StreamServerInterceptor(limiter)))
	s := grpc.NewServer(opts...)
	hello.RegisterHelloServer(s, &Hello{})
	log.Println("Listen on " + address)

	// 注册服务到 Polaris
	register := &polaris.PolarisRegister{
		Namespace:         namespace,
		Service:           service,
		Host:              ip,
		Port:              port,
		HeartbeatInterval: time.Duration(heartIntervalSec * int(time.Second)),
		ProviderAPI:       provider,
	}

	// 发送心跳
	go register.RegisterAndHeartbeat()

	// 监听服务
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := s.Serve(listen); err != nil {
			log.Fatalf("Failed to serve: %v", err)
			cancel()
		}
	}()

	// 监听退出信号，反注册并停止 grpc server
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		log.Printf("grpc server is down\n")
	case <-quit:
		log.Printf("grpc server is existing, deregister service... ...\n")
		register.DeRegister()
		s.Stop()
	}

}

//解析启动参数
func processArgs() (string, string, string, int, int) {
	// 解析启动参数
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) < 5 {
		log.Fatalf("using %s <namespace> <service>  "+
			"<ip> <port> <heartIntervalSec> ", os.Args[0])
	}
	var err error
	namespace := argsWithoutProg[0]
	service := argsWithoutProg[1]
	ip := argsWithoutProg[2]
	port, err := strconv.Atoi(argsWithoutProg[3])
	if nil != err {
		log.Fatalf("fail to convert port %s to int, err %v", argsWithoutProg[3], err)
	}
	heartIntervalSec, err := strconv.Atoi(argsWithoutProg[4])
	if nil != err {
		log.Fatalf("fail to convert heartInterval %s to int, err %v", argsWithoutProg[4], err)
	}

	return namespace, service, ip, port, heartIntervalSec
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
