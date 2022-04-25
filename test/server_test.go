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

package test

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"gopkg.in/check.v1"

	polaris "github.com/polarismesh/grpc-go-polaris"
	"github.com/polarismesh/grpc-go-polaris/test/hello"
	"github.com/polarismesh/grpc-go-polaris/test/mock"
)

const (
	serverNamespace   = "Test"
	serverSvc         = "ServerService"
	serverTestingPort = 8088
)

type serverTestingSuite struct {
	mockServer mock.NamingServer
}

func (s *serverTestingSuite) SetUpSuite(c *check.C) {
	s.mockServer = mock.NewNamingServer()
	go func() {
		err := s.mockServer.ListenAndServe(serverTestingPort)
		if nil != err {
			log.Fatal()
		}
	}()
	polaris.PolarisConfig().GetGlobal().GetServerConnector().SetAddresses(
		[]string{fmt.Sprintf("127.0.0.1:%d", serverTestingPort)})
	polaris.PolarisConfig().GetConsumer().GetLocalCache().SetPersistAvailableInterval(1 * time.Millisecond)
	polaris.PolarisConfig().GetConsumer().GetLocalCache().SetStartUseFileCache(false)
}

// 销毁套件
func (s *serverTestingSuite) TearDownSuite(c *check.C) {
	s.mockServer.Terminate()
}

type helloServer struct {
}

func (t *helloServer) SayHello(ctx context.Context, request *hello.HelloRequest) (*hello.HelloResponse, error) {
	return &hello.HelloResponse{Message: request.Name}, nil
}

func (s *serverTestingSuite) TestRegister(c *check.C) {
	srv := grpc.NewServer()
	hello.RegisterHelloServer(srv, &helloServer{})
	// 监听端口
	address := "127.0.0.1:8988"
	listen, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to addr %s: %v", address, err)
	}
	pSrv, err := polaris.Register(srv, listen,
		polaris.WithServiceName(serverSvc), polaris.WithServerNamespace(serverNamespace), polaris.WithTTL(2))
	if nil != err {
		log.Fatal(err)
	}
	defer pSrv.Deregister()
	go func() {
		err = srv.Serve(listen)
		if nil != err {
			c.Fatal(err)
		}
	}()
	time.Sleep(10 * time.Second)
	hbCount := s.mockServer.HeartbeatCount(address)
	c.Check(hbCount > 0, check.Equals, true)
}
