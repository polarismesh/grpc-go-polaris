# grpc go polaris quick start project

English | [简体中文](./README-zh.md)

Provide grpc client, server application examples to demonstrate how grpc applications can quickly connect to Polaris.

## Catalog Introduction

- rpcServer: In the server demo, an example of grpc server is provided. At the same time, connect to Polaris after startup, register for the service, and send a heartbeat to maintain a healthy state, and de-register when the application exits.
- rpcClient: An example on the client side, calling the grpc service in the server. Find the address of the server through the Polaris service.
- model: The definition of pb used in the example.

## How to build

Rely on go mod to build.

build server：
```shell
cd rpcServer
go build -o server
```

build client：

```shell
cd rpcClient
go build -o client
```


## How to use

### Create service

Create the corresponding service through the Polaris console in advance. If it is installed through a local one-click installation package, open the console directly in the browser through `127.0.0.1:8090`.

![img.png](../../doc/create_service.png)

### Modify setting

Modify the configuration and fill in the address of the Polaris server.

```yaml
global:
  serverConnector:
    addresses:
    - 127.0.0.1:8091
```

### Execute program

Run server：
```shell
./server default DemoService 127.0.0.1 9090 2
```
Start parameter explanation：
- The namespace of the registered service.
- Registered service name.
- Registered service instance ip.
- Registered service instance port.
- The interval at which heartbeats are reported.

Run client：

```shell
./client default DemoService 5 1
```
Start parameter explanation：
- The namespace which the client calls the service。
- Client calling service name.
- The number of requests sent.
- The interval at which the client synchronizes service instances from Polaris.