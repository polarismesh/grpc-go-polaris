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
	"log"
	"net"

	polaris "github.com/polarismesh/grpc-go-polaris"
	"github.com/polarismesh/grpc-go-polaris/examples/quickstart/pb"
)

// EchoService gRPC echo service struct
type EchoService struct{}

// Echo gRPC testing method
func (h *EchoService) Echo(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	return &pb.EchoResponse{Value: "echo: " + req.Value}, nil
}

func main() {
	srv := polaris.NewServer(polaris.WithServerApplication("EchoService"))
	pb.RegisterEchoServerServer(srv.GRPCServer(), &EchoService{})
	// 监听端口
	address := "0.0.0.0:0"
	listen, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to addr %s: %v", address, err)
	}
	err = srv.Serve(listen)
	if nil != err {
		log.Fatal(err)
	}
}
