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
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	_ "github.com/polarismesh/grpc-go-polaris"
	"github.com/polarismesh/grpc-go-polaris/examples/common/pb"
)

const (
	listenPort   = 0
	defaultCount = 20
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := grpc.DialContext(ctx, "polaris://VersionEchoServerGRPC", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	address := fmt.Sprintf("0.0.0.0:%d", listenPort)
	listen, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen addr %s: %v", address, err)
	}
	listenAddr := listen.Addr().String()
	fmt.Printf("listen address is %s\n", listenAddr)

	echoClient := pb.NewEchoServerClient(conn)
	echoHandler := &EchoHandler{
		echoClient: echoClient,
		ctx:        ctx,
	}
	if err := http.Serve(listen, echoHandler); nil != err {
		log.Fatal(err)
	}
}

// EchoHandler is a http.Handler that implements the echo service.
type EchoHandler struct {
	echoClient pb.EchoServerClient

	ctx context.Context
}

func (s *EchoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if nil != err {
		log.Printf("fail to parse request form: %v\n", err)
		w.WriteHeader(500)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	values := r.Form["value"]
	log.Printf("receive value is %s\n", values)
	var value string
	if len(values) > 0 {
		value = values[0]
	}

	grays := r.Form["gray"]
	log.Printf("receive gray is %s\n", grays)
	var gray string
	if len(grays) > 0 {
		gray = grays[0]
	}

	counts := r.Form["count"]
	log.Printf("receive count is %s\n", counts)
	count := defaultCount
	if len(counts) > 0 {
		v, err := strconv.Atoi(counts[0])
		if nil != err {
			log.Printf("parse count value %s into int fail, err: %s", counts[0], err)
		}
		if v > 0 {
			count = v
		}
	}
	var md metadata.MD
	if len(gray) > 0 {
		md = metadata.Pairs("gray", gray)
	}
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	builder := strings.Builder{}
	for i := 0; i < count; i++ {
		resp, err := s.echoClient.Echo(ctx, &pb.EchoRequest{Value: value})
		log.Printf("%d, send message %s, resp (%v), err(%v)\n", i, value, resp, err)
		if nil != err {
			builder.Write([]byte(err.Error()))
			builder.WriteByte('\n')
			continue
		}
		builder.Write([]byte(resp.GetValue()))
		builder.WriteByte('\n')
	}
	w.WriteHeader(200)
	_, _ = w.Write([]byte(builder.String()))
	time.Sleep(100 * time.Millisecond)

}
