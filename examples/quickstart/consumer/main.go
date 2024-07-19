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
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	polaris "github.com/polarismesh/grpc-go-polaris"
	"github.com/polarismesh/grpc-go-polaris/examples/common/pb"
)

const (
	listenPort = 18080
)

func main() {
	// grpc客户端连接获取
	ctx, cancel := context.WithCancel(context.Background())
	polaris.GetLogger().SetLevel(polaris.LogDebug)

	conn, err := polaris.DialContext(ctx, "polaris://QuickStartEchoServerGRPC",
		polaris.WithGRPCDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
		polaris.WithDisableRouter(),
		polaris.WithDisableCircuitBreaker(),
	)
	if err != nil {
		log.Fatal(err)
	}
	echoClient := pb.NewEchoServerClient(conn)

	indexHandler := func(w http.ResponseWriter, r *http.Request) {
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

		ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})
		ctx = metadata.AppendToOutgoingContext(ctx, "uid", r.Header.Get("uid"))

		// 请求时设置本次请求的负载均衡算法
		ctx = polaris.RequestScopeLbPolicy(ctx, api.LBPolicyRingHash)
		ctx = polaris.RequestScopeLbHashKey(ctx, r.Header.Get("uid"))
		resp, err := echoClient.Echo(ctx, &pb.EchoRequest{Value: value})
		log.Printf("send message, resp (%v), err(%v)", resp, err)
		if nil != err {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp.GetValue()))
	}
	http.HandleFunc("/echo", indexHandler)
	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%d", listenPort), nil); nil != err {
			log.Fatal(err)
		}
	}()
	runMainLoop(conn, cancel)
}

func runMainLoop(conn *grpc.ClientConn, cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, []os.Signal{
		syscall.SIGINT, syscall.SIGTERM,
		syscall.SIGSEGV,
	}...)

	for s := range ch {
		log.Printf("catch signal(%+v), stop servers", s)
		cancel()
		conn.Close()
		polaris.ClosePolarisContext()
		return
	}
}
