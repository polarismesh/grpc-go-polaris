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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/grpc"

	polaris "github.com/polarismesh/grpc-go-polaris"
	hello "github.com/polarismesh/grpc-go-polaris/examples/quickstart/model/grpc"
)

// go build -mod=vendor
// 命令行格式：./rpcClient <target> <sendCount> <sendInterval> <metadata k1:metadata v1,metadata k2:metadata v2,...>
// ./rpcServer Development/yourService sendCount sendInterval yourMetadata

func main() {
	target, sendCount, sendInterval := processArgs()

	//使用配置获取 Polaris SDK 对象
	//Polaris Consumer API
	consumer, err := api.NewConsumerAPI()
	if err != nil {
		log.Fatalf("api.NewConsumerAPIByConfig err(%v)", err)
	}
	defer consumer.Destroy()

	//初始化并注册 Polaris Resolver Builder
	polaris.Init(polaris.Conf{
		PolarisConsumer: consumer,
		SyncInterval:    time.Second * time.Duration(sendInterval),
	})

	//grpc客户端连接获取
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := grpc.DialContext(ctx, fmt.Sprintf("polaris://%s", target),
		[]grpc.DialOption{
			grpc.WithInsecure(),
		}...)
	if err != nil {
		panic(err)
	}

	//grpc客户端调用
	rpcClient := hello.NewHelloClient(conn)
	for i := 0; i < sendCount; i++ {
		resp, err := rpcClient.SayHello(ctx, &hello.HelloRequest{Name: "polaris"})
		log.Printf("send message, index (%d) resp1 (%v), err(%v)", i, resp, err)

		<-time.After(1500 * time.Millisecond)
	}
}

//解析启动参数
func processArgs() (string, int, int) {
	params := os.Args[1:]
	if len(params) < 3 {
		log.Fatalf("using %s <target> <sendCount> <sendInterval> ", os.Args[0])
	}

	target := params[0]
	sendCount, err := strconv.Atoi(params[1])
	if nil != err {
		log.Fatalf("fail to convert sendCount %s to int, err %v", params[1], err)
	}
	sendInterval, err := strconv.Atoi(params[2])
	if nil != err {
		log.Fatalf("fail to convert sendInterval %s to int, err %v", params[2], err)
	}

	return target, sendCount, sendInterval
}

//解析服务元数据
func parseMetadata(metadataStr string) (map[string]string, error) {
	strMetadata := strings.Split(metadataStr, ",")
	metadata := make(map[string]string, len(strMetadata))
	for _, str := range strMetadata {
		if len(str) == 0 {
			continue
		}
		metadataKv := strings.Split(str, ":")
		if len(metadataKv) != 2 {
			return nil, fmt.Errorf("invalid kv pair str %s", str)
		}
		metadata[metadataKv[0]] = metadataKv[1]
	}
	return metadata, nil
}
