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

package mock

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/uuid"
	"github.com/polarismesh/polaris-go/pkg/model"
	namingpb "github.com/polarismesh/polaris-go/pkg/model/pb/v1"
	"google.golang.org/grpc"
)

var (
	namingTypeReqToResp = map[namingpb.DiscoverRequest_DiscoverRequestType]namingpb.DiscoverResponse_DiscoverResponseType{
		namingpb.DiscoverRequest_UNKNOWN:     namingpb.DiscoverResponse_UNKNOWN,
		namingpb.DiscoverRequest_ROUTING:     namingpb.DiscoverResponse_ROUTING,
		namingpb.DiscoverRequest_CLUSTER:     namingpb.DiscoverResponse_CLUSTER,
		namingpb.DiscoverRequest_INSTANCE:    namingpb.DiscoverResponse_INSTANCE,
		namingpb.DiscoverRequest_RATE_LIMIT:  namingpb.DiscoverResponse_RATE_LIMIT,
		namingpb.DiscoverRequest_MESH_CONFIG: namingpb.DiscoverResponse_MESH_CONFIG,
		namingpb.DiscoverRequest_MESH:        namingpb.DiscoverResponse_MESH,
		namingpb.DiscoverRequest_SERVICES:    namingpb.DiscoverResponse_SERVICES,
	}
)

// NamingServer mock naming server interface
type NamingServer interface {
	namingpb.PolarisGRPCServer
	// HeartbeatCount get the hb count from mock naming server
	HeartbeatCount(address string) int
	// ListenAndServe start the server
	ListenAndServe(int) error
	// Terminate terminate the server
	Terminate()
}

type namingServer struct {
	mutex           sync.Mutex
	services        map[model.ServiceKey]map[string]*namingpb.Instance
	heartbeatCounts map[string]int
	grpcServer      *grpc.Server
}

// NewNamingServer constructor for NamingServer
func NewNamingServer() NamingServer {
	return &namingServer{
		services:        make(map[model.ServiceKey]map[string]*namingpb.Instance),
		heartbeatCounts: make(map[string]int),
	}
}

// ReportClient report client message
func (n *namingServer) ReportClient(ctx context.Context, req *namingpb.Client) (*namingpb.Response, error) {
	svrLocation := &namingpb.Location{}
	respClient := &namingpb.Client{
		Host:     req.Host,
		Type:     req.Type,
		Version:  req.Version,
		Location: svrLocation,
	}
	return &namingpb.Response{
		Code:   &wrappers.UInt32Value{Value: namingpb.ExecuteSuccess},
		Info:   &wrappers.StringValue{Value: "execute success"},
		Client: respClient,
	}, nil
}

// RegisterInstance register service instances
func (n *namingServer) RegisterInstance(ctx context.Context, instance *namingpb.Instance) (*namingpb.Response, error) {
	namespace := instance.GetNamespace().GetValue()
	service := instance.GetService().GetValue()
	host := instance.GetHost().GetValue()
	port := instance.GetPort().GetValue()
	svcKey := model.ServiceKey{
		Namespace: namespace,
		Service:   service,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	n.mutex.Lock()
	defer n.mutex.Unlock()
	instances, ok := n.services[svcKey]
	if !ok {
		instances = make(map[string]*namingpb.Instance)
		n.services[svcKey] = instances
	}
	instance.Id = &wrappers.StringValue{Value: uuid.New().String()}
	instance.Healthy = &wrappers.BoolValue{Value: true}
	instance.Weight = &wrappers.UInt32Value{Value: 100}
	instance.Revision = &wrappers.StringValue{Value: uuid.New().String()}
	instance.Isolate = &wrappers.BoolValue{Value: false}
	instances[addr] = instance
	return &namingpb.Response{
		Code:      &wrappers.UInt32Value{Value: namingpb.ExecuteSuccess},
		Namespace: &namingpb.Namespace{Name: instance.GetNamespace()},
		Service:   &namingpb.Service{Name: instance.GetService(), Namespace: instance.GetNamespace()},
		Instance:  instance,
	}, nil
}

// DeregisterInstance deregister service instances
func (n *namingServer) DeregisterInstance(ctx context.Context, instance *namingpb.Instance) (*namingpb.Response, error) {
	namespace := instance.GetNamespace().GetValue()
	service := instance.GetService().GetValue()
	host := instance.GetHost().GetValue()
	port := instance.GetPort().GetValue()
	svcKey := model.ServiceKey{
		Namespace: namespace,
		Service:   service,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	n.mutex.Lock()
	defer n.mutex.Unlock()
	instances, ok := n.services[svcKey]
	if !ok {
		instances = make(map[string]*namingpb.Instance)
		n.services[svcKey] = instances
	}
	oldInstance, ok := instances[addr]
	if !ok {
		return &namingpb.Response{
			Code:      &wrappers.UInt32Value{Value: namingpb.ExecuteSuccess},
			Namespace: &namingpb.Namespace{Name: instance.GetNamespace()},
			Service:   &namingpb.Service{Name: instance.GetService(), Namespace: instance.GetNamespace()},
		}, nil
	}
	delete(instances, addr)
	return &namingpb.Response{
		Code:      &wrappers.UInt32Value{Value: namingpb.ExecuteSuccess},
		Namespace: &namingpb.Namespace{Name: instance.GetNamespace()},
		Service:   &namingpb.Service{Name: instance.GetService(), Namespace: instance.GetNamespace()},
		Instance:  oldInstance,
	}, nil
}

// Discover unique discover method
func (n *namingServer) Discover(stream namingpb.PolarisGRPC_DiscoverServer) error {
	for {
		req, err := stream.Recv()
		if nil != err {
			if io.EOF == err {
				log.Printf("Discover: server receive eof\n")
				return nil
			}
			log.Printf("Discover: server recv error %v\n", err)
			return err
		}
		//log.Printf("Discover: server recv request %v\n", req)
		var resp *namingpb.DiscoverResponse
		resp = &namingpb.DiscoverResponse{
			Type:    namingTypeReqToResp[req.Type],
			Code:    &wrappers.UInt32Value{Value: namingpb.ExecuteSuccess},
			Service: req.Service,
		}
		if req.Type == namingpb.DiscoverRequest_INSTANCE {
			n.mutex.Lock()
			key := model.ServiceKey{
				Namespace: req.GetService().GetNamespace().GetValue(),
				Service:   req.GetService().GetName().GetValue(),
			}
			instances, ok := n.services[key]
			if ok {
				var values []*namingpb.Instance
				for _, instance := range instances {
					values = append(values, instance)
				}
				resp.Instances = values
			}
			n.mutex.Unlock()
		}
		//log.Printf("send resp, type %v, %v, resp %v", req.Type, req.Service, resp)
		if err = stream.Send(resp); nil != err {
			log.Printf("send resp err: %v", err)
			return err
		}
	}
}

// Heartbeat gRPC heartbeat method
func (n *namingServer) Heartbeat(ctx context.Context, instance *namingpb.Instance) (*namingpb.Response, error) {
	namespace := instance.GetNamespace().GetValue()
	service := instance.GetService().GetValue()
	host := instance.GetHost().GetValue()
	port := instance.GetPort().GetValue()
	svcKey := model.ServiceKey{
		Namespace: namespace,
		Service:   service,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	n.mutex.Lock()
	defer n.mutex.Unlock()
	instances, ok := n.services[svcKey]
	if !ok {
		instances = make(map[string]*namingpb.Instance)
		n.services[svcKey] = instances
	}
	_, ok = instances[addr]
	if !ok {
		return &namingpb.Response{
			Code:      &wrappers.UInt32Value{Value: namingpb.NotFoundInstance},
			Namespace: &namingpb.Namespace{Name: instance.GetNamespace()},
			Service:   &namingpb.Service{Name: instance.GetService(), Namespace: instance.GetNamespace()},
		}, nil
	}
	n.heartbeatCounts[addr] = n.heartbeatCounts[addr] + 1
	return &namingpb.Response{
		Code:      &wrappers.UInt32Value{Value: namingpb.ExecuteSuccess},
		Namespace: &namingpb.Namespace{Name: instance.GetNamespace()},
		Service:   &namingpb.Service{Name: instance.GetService(), Namespace: instance.GetNamespace()},
	}, nil
}

// HeartbeatCount get the hb count from mock naming server
func (n *namingServer) HeartbeatCount(address string) int {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	return n.heartbeatCounts[address]
}

// ListenAndServe start the server
func (n *namingServer) ListenAndServe(port int) error {
	n.grpcServer = grpc.NewServer()
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if nil != err {
		return err
	}
	namingpb.RegisterPolarisGRPCServer(n.grpcServer, n)
	return n.grpcServer.Serve(grpcListener)
}

// Terminate terminate the server
func (n *namingServer) Terminate() {
	n.grpcServer.Stop()
}
