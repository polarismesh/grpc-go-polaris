# gRPC-Go-Polaris

English | [简体中文](./README-zh.md)

## Introduction

gRPC-Go-Polaris provides a series of components based on gRPC-Go framework, developers can use gRPC-Go-Polaris to build distributed gRPC-Go applications.

## Key Features

* **Service Registration and Heartbeat**: To register the gRPC Service and send heartbeat periodly.
* **Service Routing and LoadBalancing**: Implement gRPC resover and balancer, providing semantic rule routing and loadbalacing cases.
* **Fault node circuitbreaking**: Kick of the unhealthy nodes when loadbalacing, base on the service invoke successful rate.
* **RateLimiting**: Implement gRPC interceptor, providing request ratelimit check to ensure the reliability for server.

## Base Architecture

![arch](doc\arch.png)
gRPC-Go-Polaris implements the interfaces on gRPC-Go, to access polarismesh functions.

## How To Use

### Prerequisites

- **[Go][]**: any one of the **three latest major** [releases][go-releases].

### Installation

With [Go module][] support (Go 1.11+), simply add the following import

```go
import "github.com/polarismesh/grpc-go-polaris"
```

to your code, and then `go [build|run|test]` will automatically fetch the
necessary dependencies.

Otherwise, to install the `grpc-go-polaris` package, run the following command:

```console
$ go get -u github.com/polarismesh/grpc-go-polaris
```

> **Note:** gRPC-Go-Polaris has `gRPC-Go` dependencies, it will encounter timeout while go getting `gRPC-Go` in China, to solution the problem, you can refer [FAQ](https://github.com/grpc/grpc-go#FAQ).

### Examples

- [QuickStart](examples/quickstart/README.md)