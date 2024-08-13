# gRPC-Go-Polaris 快速入门

[English](./README.md) | 简体中文

提供基于gRPC框架开发的应用示例（包含consumer和provider两个应用），演示使用 gRPC 框架开发的应用如何快速接入北极星。

## 目录介绍

- provider: gRPC 服务端示例，演示服务实例注册、反注册、以及心跳上报保活的功能。
- consumer: gRPC 客户端示例，演示服务发现、负载均衡、以及服务调用的功能。

## 使用说明

### 修改配置

在 ```provider``` 以及 ```consumer``` 两个项目中，修改```polaris.yaml```，修改后配置如下所示。
其中，```${ip}```和```${port}```为Polaris后端服务的IP地址与端口号。

```yaml
global:
  serverConnector:
    addresses:
      - ${ip}:${port}
```

## 如何构建

依赖 go mod 进行构建。

构建 provider：

```shell
cd provider
go build -o provider
```

构建 consumer：

```shellq
cd consumer
go build -o consumer
```

## 启动样例

### 启动Provider

go mod 编译打包：

```shell
cd provider
go build -o provider
```

然后运行生成的二进制文件：

```shell
./provider
```

### 启动Consumer

go mod 编译打包：

```shell
cd consumer
go build -o consumer
```

然后运行生成的二进制文件：

```shell
./consumer
```

### 验证

#### 控制台验证

登录polaris控制台，可以看到EchoServerGRPC服务下存在对应的provider实例。

#### HTTP调用

执行http调用，其中`${app.port}`替换为consumer的监听端口（默认为 18080 ）。

```shell
curl -H 'uid: 12313' -L -X GET 'http://localhost:${app.port}/echo?value=hello_world'
```

预期返回值：`echo: hello_world`