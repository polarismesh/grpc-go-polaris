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
	"google.golang.org/grpc/grpclog"
	"google.golang.org/protobuf/proto"
)

// Server encapsulated server with gRPC option
type Server struct {
	*grpc.Server
	serverOptions    serverOptions
	sdkCtx           api.SDKContext
	providerAPI      api.ProviderAPI
	registerRequests []*api.InstanceRegisterRequest
}

// NewServer start polaris server
func NewServer(opts ...ServerOption) (*Server, error) {
	srv := &Server{}
	if err := srv.initResource(opts...); err != nil {
		return nil, err
	}

	if srv.serverOptions.enableRatelimit != nil && *srv.serverOptions.enableRatelimit {
		limiter := newRateLimitInterceptor(srv.sdkCtx).
			WithNamespace(srv.serverOptions.namespace).
			WithServiceName(srv.serverOptions.svcName)
		// 添加北极星限流的 gRPC Interceptor
		srv.serverOptions.gRPCServerOptions = append(srv.serverOptions.gRPCServerOptions,
			grpc.ChainUnaryInterceptor(limiter.UnaryInterceptor),
		)
	}

	gSrv := grpc.NewServer(srv.serverOptions.gRPCServerOptions...)
	srv.Server = gSrv
	return srv, nil
}

func (srv *Server) initResource(opts ...ServerOption) error {
	srv.serverOptions = serverOptions{}

	for _, opt := range opts {
		opt.apply(&srv.serverOptions)
	}
	srv.serverOptions.setDefault()

	if *srv.serverOptions.delayRegisterEnable {
		delayStrategy := srv.serverOptions.delayRegisterStrategy
		for {
			if delayStrategy.Allow() {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	srv.sdkCtx = srv.serverOptions.sdkCtx
	srv.providerAPI = api.NewProviderAPIByContext(srv.sdkCtx)
	return nil
}

func (srv *Server) doRegister(lis net.Listener) error {
	if len(srv.serverOptions.host) == 0 {
		host, err := getLocalHost(srv.sdkCtx.GetConfig().GetGlobal().GetServerConnector().GetAddresses()[0])
		if nil != err {
			return fmt.Errorf("error occur while fetching localhost: %w", err)
		}
		srv.serverOptions.host = host
	}
	port, err := parsePort(lis.Addr().String())
	if nil != err {
		return fmt.Errorf("error occur while parsing port from listener: %w", err)
	}
	srv.serverOptions.port = port
	svcInfos := buildServiceNames(srv.Server, srv)

	for _, name := range svcInfos {
		registerRequest := buildRegisterInstanceRequest(srv, name)
		srv.registerRequests = append(srv.registerRequests, registerRequest)
		resp, err := srv.providerAPI.RegisterInstance(registerRequest)
		if nil != err {
			deregisterServices(srv.providerAPI, srv.registerRequests)
			return fmt.Errorf("fail to register service %s: %w", name, err)
		}
		grpclog.Infof("[Polaris][Naming] success to register %s:%d to service %s(%s), id %s",
			registerRequest.Host, registerRequest.Port, name, registerRequest.Namespace, resp.InstanceID)
	}
	return nil
}

// Serve 代理 gRPC Server 的 Serve
func (srv *Server) Serve(lis net.Listener) error {
	if err := srv.doRegister(lis); err != nil {
		return err
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		s := <-c
		log.Printf("[Polaris][Naming] receive quit signal: %v", s)
		signal.Stop(c)
		srv.Stop()
	}()

	return srv.Server.Serve(lis)
}

// Stop deregister and stop
func (srv *Server) Stop() {
	srv.Deregister()

	if !*srv.serverOptions.gracefulStopEnable {
		srv.Server.Stop()
		return
	}

	ctx, cancel := context.WithDeadline(context.Background(),
		time.Now().Add(srv.serverOptions.gracefulStopMaxWaitDuration))
	go func() {
		srv.Server.GracefulStop()
		cancel()
	}()

	<-ctx.Done()
}

// Deregister deregister services from polaris
func (srv *Server) Deregister() {
	deregisterServices(srv.providerAPI, srv.registerRequests)
}

// Serve start polaris server
func Serve(gSrv *grpc.Server, lis net.Listener, opts ...ServerOption) error {
	pSrv, err := Register(gSrv, lis, opts...)
	if err != nil {
		log.Fatalf("polaris register err: %v", err)
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		s := <-c
		log.Printf("[Polaris][Naming] receive quit signal: %v", s)
		signal.Stop(c)
		pSrv.Stop()
	}()

	return gSrv.Serve(lis)
}

// Register server as polaris instances
func Register(gSrv *grpc.Server, lis net.Listener, opts ...ServerOption) (*Server, error) {
	srv := &Server{Server: gSrv}
	if err := srv.initResource(opts...); err != nil {
		return nil, err
	}
	return srv, srv.doRegister(lis)
}

func buildServiceNames(gSrv *grpc.Server, svr *Server) []string {
	svcInfo := gSrv.GetServiceInfo()
	ret := make([]string, 0, len(svcInfo))
	for k := range svcInfo {
		ret = append(ret, k)
	}

	if len(svr.serverOptions.svcName) != 0 {
		ret = []string{
			svr.serverOptions.svcName,
		}
	}

	return ret
}

func buildRegisterInstanceRequest(srv *Server, serviceName string) *api.InstanceRegisterRequest {
	registerRequest := &api.InstanceRegisterRequest{}
	registerRequest.Namespace = srv.serverOptions.namespace
	registerRequest.Service = serviceName
	registerRequest.Host = srv.serverOptions.host
	registerRequest.Port = srv.serverOptions.port
	registerRequest.Protocol = proto.String("grpc")
	registerRequest.Metadata = srv.serverOptions.metadata
	registerRequest.Version = proto.String(srv.serverOptions.version)
	registerRequest.ServiceToken = srv.serverOptions.token
	registerRequest.SetTTL(srv.serverOptions.ttl)
	return registerRequest
}

func getLocalHost(serverAddr string) (string, error) {
	conn, err := net.Dial("tcp", serverAddr)
	if nil != err {
		return "", err
	}
	localAddr := conn.LocalAddr().String()
	colonIdx := strings.LastIndex(localAddr, ":")
	if colonIdx > 0 {
		return localAddr[:colonIdx], nil
	}
	return localAddr, nil
}

func parsePort(addr string) (int, error) {
	colonIdx := strings.LastIndex(addr, ":")
	if colonIdx < 0 {
		return 0, fmt.Errorf("invalid addr string: %s", addr)
	}
	portStr := addr[colonIdx+1:]
	return strconv.Atoi(portStr)
}

func deregisterServices(providerAPI api.ProviderAPI, services []*api.InstanceRegisterRequest) {
	if len(services) == 0 {
		return
	}
	for _, registerRequest := range services {
		deregisterRequest := &api.InstanceDeRegisterRequest{}
		deregisterRequest.Namespace = registerRequest.Namespace
		deregisterRequest.Service = registerRequest.Service
		deregisterRequest.Host = registerRequest.Host
		deregisterRequest.Port = registerRequest.Port
		deregisterRequest.ServiceToken = registerRequest.ServiceToken
		err := providerAPI.Deregister(deregisterRequest)
		if nil != err {
			grpclog.Errorf("[Polaris][Naming] fail to deregister %s:%d to service %s(%s)",
				deregisterRequest.Host, deregisterRequest.Port, deregisterRequest.Service, deregisterRequest.Namespace)
			continue
		}
		grpclog.Infof("[Polaris][Naming] success to deregister %s:%d to service %s(%s)",
			deregisterRequest.Host, deregisterRequest.Port, deregisterRequest.Service, deregisterRequest.Namespace)
	}
}
