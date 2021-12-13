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
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

// Server encapsulated server with gRPC option
type Server struct {
	gRPCServer    *grpc.Server
	serverOptions serverOptions
}

type serverOptions struct {
	gRPCServerOptions []grpc.ServerOption
	namespace         string
	application       string
	ttl               int
	metadata          map[string]string
	host              string
	port              int
}

func (s *serverOptions) setDefault() {
	if len(s.namespace) == 0 {
		s.namespace = defaultNamespace
	}
	if s.ttl == 0 {
		s.ttl = defaultTTL
	}
}

// A ServerOption sets options such as credentials, codec and keepalive parameters, etc.
type ServerOption interface {
	apply(*serverOptions)
}

// funcServerOption wraps a function that modifies serverOptions into an
// implementation of the ServerOption interface.
type funcServerOption struct {
	f func(*serverOptions)
}

func (fdo *funcServerOption) apply(do *serverOptions) {
	fdo.f(do)
}

func newFuncServerOption(f func(*serverOptions)) *funcServerOption {
	return &funcServerOption{
		f: f,
	}
}

// WithServerApplication set the application to register instance
func WithServerApplication(application string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.application = application
	})
}

// WithGRPCServerOptions set the raw gRPC serverOptions
func WithGRPCServerOptions(opts ...grpc.ServerOption) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.gRPCServerOptions = opts
	})
}

// WithServerNamespace set the namespace to register instance
func WithServerNamespace(namespace string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.namespace = namespace
	})
}

// WithServerMetadata set the metadata to register instance
func WithServerMetadata(metadata map[string]string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.metadata = metadata
	})
}

// WithServerHost set the host to register instance
func WithServerHost(host string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.host = host
	})
}

// WithTTL set the ttl to register instance
func WithTTL(ttl int) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.ttl = ttl
	})
}

// WithPort set the port to register instance
func WithPort(port int) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.port = port
	})
}

// NewServer initializer for gRPC server
func NewServer(opts ...ServerOption) *Server {
	srv := &Server{}
	for _, opt := range opts {
		opt.apply(&srv.serverOptions)
	}
	srv.serverOptions.setDefault()
	srv.gRPCServer = grpc.NewServer(srv.serverOptions.gRPCServerOptions...)
	return srv
}

// GRPCServer get the raw gRPC server
func (s *Server) GRPCServer() *grpc.Server {
	return s.gRPCServer
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

func deregisterServices(registerContext *RegisterContext) {
	registerContext.cancel()
	if nil != registerContext.healthCheckWait {
		grpclog.Infof("[Polaris]start to wait heartbeat finish")
		registerContext.healthCheckWait.Wait()
		grpclog.Infof("[Polaris]success to wait heartbeat finish")
	}
	if len(registerContext.registerRequests) == 0 {
		return
	}
	for _, registerRequest := range registerContext.registerRequests {
		deregisterRequest := &api.InstanceDeRegisterRequest{}
		deregisterRequest.Namespace = registerRequest.Namespace
		deregisterRequest.Service = registerRequest.Service
		deregisterRequest.Host = registerRequest.Host
		deregisterRequest.Port = registerRequest.Port
		err := registerContext.providerAPI.Deregister(deregisterRequest)
		if nil != err {
			grpclog.Errorf("[Polaris]fail to deregister %s:%d to service %s(%s)",
				deregisterRequest.Host, deregisterRequest.Port, deregisterRequest.Service, deregisterRequest.Namespace)
			continue
		}
		grpclog.Infof("[Polaris]success to deregister %s:%d to service %s(%s)",
			deregisterRequest.Host, deregisterRequest.Port, deregisterRequest.Service, deregisterRequest.Namespace)
	}
}

// RegisterContext context parameters by register
type RegisterContext struct {
	providerAPI      api.ProviderAPI
	registerRequests []*api.InstanceRegisterRequest
	cancel           context.CancelFunc
	healthCheckWait  *sync.WaitGroup
}

const maxHeartbeatIntervalSec = 60

func (s *Server) startHeartbeat(ctx context.Context,
	providerAPI api.ProviderAPI, registerRequests []*api.InstanceRegisterRequest) *sync.WaitGroup {
	heartbeatIntervalSec := s.serverOptions.ttl
	if heartbeatIntervalSec > maxHeartbeatIntervalSec {
		heartbeatIntervalSec = maxHeartbeatIntervalSec
	}
	wg := &sync.WaitGroup{}
	wg.Add(len(registerRequests))
	for i, request := range registerRequests {
		go func(idx int, registerRequest *api.InstanceRegisterRequest) {
			ticker := time.NewTicker(time.Duration(heartbeatIntervalSec) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					grpclog.Infof("[Polaris]heartbeat ticker %d has stopped")
					wg.Done()
					return
				case <-ticker.C:
					hbRequest := &api.InstanceHeartbeatRequest{}
					hbRequest.Namespace = registerRequest.Namespace
					hbRequest.Service = registerRequest.Service
					hbRequest.Host = registerRequest.Host
					hbRequest.Port = registerRequest.Port
					err := providerAPI.Heartbeat(hbRequest)
					if nil != err {
						grpclog.Errorf("[Polaris]fail to heartbeat %s:%d to service %s(%s)",
							hbRequest.Host, hbRequest.Port, hbRequest.Service, hbRequest.Namespace)
					}
				}
			}
		}(i, request)
		grpclog.Infof("[Polaris]success to schedule heartbeat for %s:%d, service %s(%s)",
			request.Host, request.Port, request.Service, request.Namespace)
	}
	return wg
}

// Serve listen and accept connections
func (s *Server) Serve(lis net.Listener) error {
	svcInfos := s.gRPCServer.GetServiceInfo()
	ctx, cancel := context.WithCancel(context.Background())
	registerContext := &RegisterContext{
		cancel: cancel,
	}
	defer deregisterServices(registerContext)
	if len(svcInfos) > 0 {
		polarisCtx, err := PolarisContext()
		if nil != err {
			return err
		}
		if len(s.serverOptions.host) == 0 {
			host, err := getLocalHost(polarisCtx.GetConfig().GetGlobal().GetServerConnector().GetAddresses()[0])
			if nil != err {
				return fmt.Errorf("error occur while fetching localhost: %v", err)
			}
			s.serverOptions.host = host
		}
		if s.serverOptions.port == 0 {
			port, err := parsePort(lis.Addr().String())
			if nil != err {
				return fmt.Errorf("error occur while parsing port from listener: %v", err)
			}
			s.serverOptions.port = port
		}

		registerContext.registerRequests = make([]*api.InstanceRegisterRequest, 0, len(svcInfos))
		registerContext.providerAPI = api.NewProviderAPIByContext(polarisCtx)
		for name := range svcInfos {
			var svcName = name
			if len(s.serverOptions.application) > 0 {
				svcName = s.serverOptions.application
			}
			registerRequest := &api.InstanceRegisterRequest{}
			registerRequest.Namespace = s.serverOptions.namespace
			registerRequest.Service = svcName
			registerRequest.Host = s.serverOptions.host
			registerRequest.Port = s.serverOptions.port
			registerRequest.SetTTL(s.serverOptions.ttl)
			registerRequest.Protocol = proto.String(lis.Addr().Network())
			registerRequest.Metadata = s.serverOptions.metadata
			registerContext.registerRequests = append(registerContext.registerRequests, registerRequest)
			resp, err := registerContext.providerAPI.Register(registerRequest)
			if nil != err {
				return fmt.Errorf("fail to register service %s: %v", name, err)
			}
			grpclog.Infof("[Polaris]success to register %s:%d to service %s(%s), id %s",
				registerRequest.Host, registerRequest.Port, name, registerRequest.Namespace, resp.InstanceID)
		}
		registerContext.healthCheckWait =
			s.startHeartbeat(ctx, registerContext.providerAPI, registerContext.registerRequests)
	}
	return s.gRPCServer.Serve(lis)
}
