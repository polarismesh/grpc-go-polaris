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
	"sync"
	"syscall"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/protobuf/proto"
)

// Server encapsulated server with gRPC option
type Server struct {
	gServer         *grpc.Server
	serverOptions   serverOptions
	registerContext *RegisterContext
}

type serverOptions struct {
	gRPCServerOptions []grpc.ServerOption
	namespace         string
	svcName           string
	heartbeatEnable   *bool
	ttl               int
	metadata          map[string]string
	host              string
	port              int
	version           string
	token             string
	ctrlOpts          ctrlOptions
}

type ctrlOptions struct {
	delayRegisterEnable         *bool
	delayRegisterStrategy       DelayStrategy
	delayStopEnable             *bool
	delayStopStrategy           DelayStrategy
	gracefulStopEnable          *bool
	gracefulStopMaxWaitDuration time.Duration
}

func (s *serverOptions) setDefault() {
	if len(s.namespace) == 0 {
		s.namespace = DefaultNamespace
	}
	if s.ttl == 0 {
		s.ttl = DefaultTTL
	}
	if s.heartbeatEnable == nil {
		setHeartbeatEnable(s, true)
	}
	if s.ctrlOpts.delayRegisterEnable == nil {
		setDelayRegisterEnable(s, false)
	}
	if *s.ctrlOpts.delayRegisterEnable {
		if s.ctrlOpts.delayRegisterStrategy == nil {
			setDelayRegisterStrategy(s, &NoopDelayStrategy{})
		}
	}
	if s.ctrlOpts.delayStopEnable == nil {
		setDelayStopEnable(s, true)
	}
	if *s.ctrlOpts.delayStopEnable {
		if s.ctrlOpts.delayStopStrategy == nil {
			setDelayStopStrategy(s, &WaitDelayStrategy{WaitTime: DefaultDelayStopWaitDuration})
		}
	}
	if s.ctrlOpts.gracefulStopEnable == nil {
		setGracefulStopEnable(s, true)
	}
	if *s.ctrlOpts.gracefulStopEnable {
		if s.ctrlOpts.gracefulStopMaxWaitDuration <= 0 {
			setGracefulStopMaxWaitDuration(s, DefaultGracefulStopMaxWaitDuration)
		}
	}
}

// DelayStrategy delay register/deregister strategy. e.g. wait some time
type DelayStrategy interface {
	allow() bool
}

// NoopDelayStrategy noop delay strategy
type NoopDelayStrategy struct{}

func (d *NoopDelayStrategy) allow() bool {
	return true
}

// WaitDelayStrategy sleep wait delay strategy
type WaitDelayStrategy struct {
	WaitTime time.Duration
}

func (d *WaitDelayStrategy) allow() bool {
	time.Sleep(d.WaitTime)
	return true
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

// WithServerApplication set application name
// Deprecated: WithServerApplication set the application to register instance
func WithServerApplication(application string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.svcName = application
	})
}

// WithServiceName set the application to register instance
func WithServiceName(svcName string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.svcName = svcName
	})
}

func setHeartbeatEnable(options *serverOptions, enable bool) {
	options.heartbeatEnable = &enable
}

// WithHeartbeatEnable enables the heartbeat task to instance
func WithHeartbeatEnable(enable bool) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setHeartbeatEnable(options, enable)
	})
}

func setDelayRegisterEnable(options *serverOptions, enable bool) {
	options.ctrlOpts.delayRegisterEnable = &enable
}

func setDelayRegisterStrategy(options *serverOptions, strategy DelayStrategy) {
	options.ctrlOpts.delayRegisterStrategy = strategy
}

// EnableDelayRegister enables delay register
func EnableDelayRegister(strategy DelayStrategy) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setDelayRegisterEnable(options, true)
		setDelayRegisterStrategy(options, strategy)
	})
}

func setDelayStopEnable(options *serverOptions, enable bool) {
	options.ctrlOpts.delayStopEnable = &enable
}

func setDelayStopStrategy(options *serverOptions, strategy DelayStrategy) {
	options.ctrlOpts.delayStopStrategy = strategy
}

// EnableDelayStop enables delay deregister
func EnableDelayStop(strategy DelayStrategy) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setDelayStopEnable(options, true)
		setDelayStopStrategy(options, strategy)
	})
}

func setGracefulStopEnable(options *serverOptions, enable bool) {
	options.ctrlOpts.gracefulStopEnable = &enable
}

func setGracefulStopMaxWaitDuration(options *serverOptions, duration time.Duration) {
	options.ctrlOpts.gracefulStopMaxWaitDuration = duration
}

// EnableGracefulStop enables graceful stop
func EnableGracefulStop(duration time.Duration) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		setGracefulStopEnable(options, true)
		setGracefulStopMaxWaitDuration(options, duration)
	})
}

// WithGRPCServerOptions set the raw gRPC serverOptions
func WithGRPCServerOptions(opts ...grpc.ServerOption) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.gRPCServerOptions = opts
	})
}

// WithToken set the token to do server operations
func WithToken(token string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.token = token
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

// WithServerVersion set the version to register instance
func WithServerVersion(version string) ServerOption {
	return newFuncServerOption(func(options *serverOptions) {
		options.version = version
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
	fmt.Printf("invoke deregisterServices\n")
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
		deregisterRequest.ServiceToken = registerRequest.ServiceToken
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
	providerAPI       api.ProviderAPI
	registerRequests  []*api.InstanceRegisterRequest
	heartbeatRequests []*api.InstanceRegisterRequest
	cancel            context.CancelFunc
	healthCheckWait   *sync.WaitGroup
}

const maxHeartbeatIntervalSec = 60

func checkAddress(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
	if nil != err {
		grpclog.Infof("[Polaris]fail to dial %s: %v", address, err)
		return false
	}
	_ = conn.Close()
	return true
}

func (s *Server) startHeartbeat(ctx context.Context,
	providerAPI api.ProviderAPI, registerRequests []*api.InstanceRegisterRequest) *sync.WaitGroup {
	heartbeatIntervalSec := s.serverOptions.ttl
	if heartbeatIntervalSec > maxHeartbeatIntervalSec {
		heartbeatIntervalSec = maxHeartbeatIntervalSec
	}
	wg := &sync.WaitGroup{}
	wg.Add(len(registerRequests))
	dialResults := make(map[string]bool)
	for i, request := range registerRequests {
		go func(idx int, registerRequest *api.InstanceRegisterRequest) {
			ticker := time.NewTicker(time.Duration(heartbeatIntervalSec) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					grpclog.Infof("[Polaris]heartbeat ticker has stopped IDX:%d", idx)
					wg.Done()
					return
				case <-ticker.C:
					address := fmt.Sprintf("%s:%d", registerRequest.Host, registerRequest.Port)
					result, ok := dialResults[address]
					if !ok {
						result = checkAddress(address)
						dialResults[address] = result
					}
					if result {
						hbRequest := &api.InstanceHeartbeatRequest{}
						hbRequest.Namespace = registerRequest.Namespace
						hbRequest.Service = registerRequest.Service
						hbRequest.Host = registerRequest.Host
						hbRequest.Port = registerRequest.Port
						hbRequest.ServiceToken = registerRequest.ServiceToken
						err := providerAPI.Heartbeat(hbRequest)
						if nil != err {
							grpclog.Errorf("[Polaris]fail to heartbeat %s:%d to service %s(%s): %v",
								hbRequest.Host, hbRequest.Port, hbRequest.Service, hbRequest.Namespace, err)
						}
					}
				}
			}
		}(i, request)
		grpclog.Infof("[Polaris]success to schedule heartbeat for %s:%d, service %s(%s)",
			request.Host, request.Port, request.Service, request.Namespace)
	}
	return wg
}

// Serve start polaris server
func Serve(gSrv *grpc.Server, lis net.Listener, opts ...ServerOption) error {
	go func() {
		pSrv, err := Register(gSrv, lis, opts...)
		if err != nil {
			log.Fatalf("polaris register err: %v", err)
		}

		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			s := <-c
			log.Printf("receive quit signal: %v", s)
			signal.Stop(c)
			pSrv.Stop()
		}()
	}()

	return gSrv.Serve(lis)
}

// Stop deregister and stop
func (s *Server) Stop() {
	s.Deregister()
	if *s.serverOptions.ctrlOpts.delayStopEnable {
		delayStrategy := s.serverOptions.ctrlOpts.delayStopStrategy
		for {
			if delayStrategy.allow() {
				break
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	if *s.serverOptions.ctrlOpts.gracefulStopEnable {
		stopped := make(chan struct{})
		go func() {
			s.gServer.GracefulStop()
			close(stopped)
		}()

		t := time.NewTimer(s.serverOptions.ctrlOpts.gracefulStopMaxWaitDuration)
		select {
		case <-t.C:
			s.gServer.Stop()
		case <-stopped:
			t.Stop()
		}
	} else {
		s.gServer.Stop()
	}
}

// Register server as polaris instances
func Register(gSrv *grpc.Server, lis net.Listener, opts ...ServerOption) (*Server, error) {
	srv := &Server{gServer: gSrv}
	for _, opt := range opts {
		opt.apply(&srv.serverOptions)
	}
	srv.serverOptions.setDefault()
	svcInfos := gSrv.GetServiceInfo()
	ctx, cancel := context.WithCancel(context.Background())
	registerContext := &RegisterContext{
		cancel: cancel,
	}
	if len(svcInfos) > 0 {
		polarisCtx, err := PolarisContext()
		if nil != err {
			return nil, err
		}
		if len(srv.serverOptions.host) == 0 {
			host, err := getLocalHost(polarisCtx.GetConfig().GetGlobal().GetServerConnector().GetAddresses()[0])
			if nil != err {
				return nil, fmt.Errorf("error occur while fetching localhost: %w", err)
			}
			srv.serverOptions.host = host
		}
		if srv.serverOptions.port == 0 {
			port, err := parsePort(lis.Addr().String())
			if nil != err {
				return nil, fmt.Errorf("error occur while parsing port from listener: %w", err)
			}
			srv.serverOptions.port = port
		}

		if *srv.serverOptions.ctrlOpts.delayRegisterEnable {
			delayStrategy := srv.serverOptions.ctrlOpts.delayRegisterStrategy
			for {
				if delayStrategy.allow() {
					break
				} else {
					time.Sleep(100 * time.Millisecond)
				}
			}
		}

		registerContext.registerRequests = make([]*api.InstanceRegisterRequest, 0, len(svcInfos))
		registerContext.providerAPI = api.NewProviderAPIByContext(polarisCtx)
		for name := range svcInfos {
			var svcName = name
			if len(srv.serverOptions.svcName) > 0 {
				svcName = srv.serverOptions.svcName
			}
			registerRequest := &api.InstanceRegisterRequest{}
			registerRequest.Namespace = srv.serverOptions.namespace
			registerRequest.Service = svcName
			registerRequest.Host = srv.serverOptions.host
			registerRequest.Port = srv.serverOptions.port
			registerRequest.Protocol = proto.String(lis.Addr().Network())
			registerRequest.Metadata = srv.serverOptions.metadata
			registerRequest.Version = proto.String(srv.serverOptions.version)
			registerRequest.ServiceToken = srv.serverOptions.token
			if *srv.serverOptions.heartbeatEnable {
				registerRequest.SetTTL(srv.serverOptions.ttl)
				registerContext.heartbeatRequests = append(registerContext.heartbeatRequests, registerRequest)
			}
			registerContext.registerRequests = append(registerContext.registerRequests, registerRequest)
			resp, err := registerContext.providerAPI.Register(registerRequest)
			if nil != err {
				deregisterServices(registerContext)
				return nil, fmt.Errorf("fail to register service %s: %w", name, err)
			}
			grpclog.Infof("[Polaris]success to register %s:%d to service %s(%s), id %s",
				registerRequest.Host, registerRequest.Port, name, registerRequest.Namespace, resp.InstanceID)
		}
		if len(registerContext.heartbeatRequests) > 0 {
			registerContext.healthCheckWait =
				srv.startHeartbeat(ctx, registerContext.providerAPI, registerContext.heartbeatRequests)
		}
	}
	srv.registerContext = registerContext
	return srv, nil
}

// Deregister deregister services from polaris
func (s *Server) Deregister() {
	deregisterServices(s.registerContext)
}
