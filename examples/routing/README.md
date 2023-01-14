# gRPC-Go-Polaris Routing example

English | [简体中文](./README-zh.md)

Provide consumer and provider applications which based on gRPC framework, to show how to make gRPC application access polaris rapidly.

## Content

- provider: gRPC server application, demo service register, deregister, heartbeat.
- consumer: gRPC client application, demo service discovery, load balance and route.

## Instruction

### Configuration

Modify ```polaris.yaml``` in ```provider``` and ```consumer```, which is showed as below:
besides, ```${ip}``` and ```${port}``` is the address of polaris server.

```yaml
global:
  serverConnector:
    addresses:
    - ${ip}:${port}
```

## How to build

Build provider with go mod:

```shell
cd provider
go build -o provider
```

Build consumer with go mod:

```shellq
cd consumer
go build -o consumer
```

## Start application

### Start Provider

go mod compile and build:
```shell
cd provider
go build -o provider
```

run binary executable：

```shell
./provider --port="20000" --labels="env=dev" > provider-20000.log 2>&1 &
./provider --port="20001" --labels="env=pre" > provider-20001.log 2>&1 &
./provider --port="20002" --labels="env=prod" > provider-20002.log 2>&1 &
```

### Start Consumer

go mod compile and build:
```shell
cd consumer
go build -o consumer
```

run binary executable：

```shell
./consumer
```

### Verify

#### Check polaris console

Login into polaris console, and check the instances in Service `RouteEchoServerGRPC`.

#### Invoke by http call

Invoke http call，replace `${app.port}` to the consumer port (16011 by default).
```shell
curl -H 'uid: 1' -L -X GET 'http://localhost:47080/echo?value=hello_world''
```

expect：`echo: hello_world, from: env:dev`