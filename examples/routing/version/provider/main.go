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
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	"google.golang.org/grpc"

	polaris "github.com/polarismesh/grpc-go-polaris"
	"github.com/polarismesh/grpc-go-polaris/examples/common/pb"
)

var (
	listenPort int
	version    string
)

func initArgs() {
	flag.StringVar(&version, "version", "", "eg. 1.0.0")
}

// EchoVersionService gRPC echo service struct
type EchoVersionService struct {
	version string
}

// Echo gRPC testing method
func (h *EchoVersionService) Echo(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	return &pb.EchoResponse{Value: fmt.Sprintf("echo: %s, from %s", req.Value, h.version)}, nil
}

func main() {
	initArgs()
	flag.Parse()
	address := fmt.Sprintf("0.0.0.0:%d", listenPort)
	listen, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to addr %s: %v", address, err)
	}
	listenAddr := listen.Addr().String()
	fmt.Printf("listen address is %s\n", listenAddr)
	srv := grpc.NewServer()
	pb.RegisterEchoServerServer(srv, &EchoVersionService{version: version})
	// 执行北极星的注册命令
	pSrv, err := polaris.Register(srv, listen,
		polaris.WithServiceName("VersionEchoServerGRPC"),
		polaris.WithServerVersion(version))
	if nil != err {
		log.Fatal(err)
	}
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c)
		s := <-c
		log.Printf("receive quit signal: %v", s)
		// 执行北极星的反注册命令
		pSrv.Deregister()
		srv.GracefulStop()
	}()
	err = srv.Serve(listen)
	if nil != err {
		log.Printf("listen err: %v", err)
	}
}
