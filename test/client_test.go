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

	polaris "github.com/polarismesh/grpc-go-polaris"
	"github.com/polarismesh/grpc-go-polaris/test/hello"
	"github.com/polarismesh/grpc-go-polaris/test/mock"
	"google.golang.org/grpc"
	"gopkg.in/check.v1"
)

type clientTestingSuite struct {
	mockServer mock.NamingServer
}

func (s *clientTestingSuite) SetUpSuite(c *check.C) {
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

func (s *clientTestingSuite) TearDownSuite(c *check.C) {
	s.mockServer.Terminate()
}

func (s *clientTestingSuite) TestClientCall(c *check.C) {
	srv := polaris.NewServer(
		polaris.WithServerApplication(serverSvc), polaris.WithServerNamespace(serverNamespace), polaris.WithTTL(2))
	hello.RegisterHelloServer(srv.GRPCServer(), &helloServer{})
	listen, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	log.Printf("success to listen on %s\n", listen.Addr())
	defer listen.Close()
	go func() {
		err = srv.Serve(listen)
		if nil != err {
			c.Fatal(err)
		}
	}()
	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := polaris.DialContext(ctx, "polaris://"+serverSvc, polaris.WithGRPCDialOptions(grpc.WithInsecure()),
		polaris.WithClientNamespace(serverNamespace))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := hello.NewHelloClient(conn)
	for i := 0; i < 5; i++ {
		_, err := client.SayHello(context.Background(), &hello.HelloRequest{Name: "hello"})
		c.Assert(err, check.IsNil)
		time.Sleep(1 * time.Second)
	}
}
