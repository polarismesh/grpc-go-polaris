package grpcpolaris

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	polarispb "github.com/polarismesh/polaris-go/pkg/model/pb"
	namingpb "github.com/polarismesh/polaris-go/pkg/model/pb/v1"

	hello "github.com/polarismesh/grpc-go-polaris/sample/model/grpc"
)

// TestMain
func TestMain(m *testing.M) {
	m.Run()
	os.Exit(0)
}

// TestPolarisResolver 黑盒测试Resolver基本功能
func TestPolarisResolver(t *testing.T) {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stdout, os.Stdout, os.Stderr, 999))
	polarisConsumer := newMockPolarisConsumer()
	Init(Conf{
		PolarisConsumer: polarisConsumer,
		SyncInterval:    time.Second * 5,
	})
	// init grpc server
	srv := grpc.NewServer()
	hello.RegisterHelloServer(srv, &Hello{})
	go func() {
		address := "0.0.0.0:8000"
		listen, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}
		err = srv.Serve(listen)
		if err != nil {
			log.Fatalf("failed to Serv:%s", err)
		}
	}()
	go func() {
		address := "0.0.0.0:8002"
		listen, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}
		err = srv.Serve(listen)
		if err != nil {
			log.Fatalf("failed to Serv:%s", err)
		}
	}()
	time.Sleep(time.Millisecond * 100)
	//  grpc client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := grpc.DialContext(ctx, "polaris://Production/grpc.service", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("failed to dial:%s", err)
	}
	rpcClient := hello.NewHelloClient(conn)
	for i := 0; i < 10; i++ {
		start := time.Now()
		resp, err := rpcClient.SayHello(ctx, &hello.HelloRequest{Name: "polaris"})
		if err != nil {
			t.Fatal(err)
		}
		log.Printf("call cost:%v, resp:%v, err:%v", time.Since(start), resp, err)
		time.Sleep(time.Second)
	}
}

// Hello is HelloService
type Hello struct{}

// SayHello implement HelloService.SayHello
func (h *Hello) SayHello(ctx context.Context, req *hello.HelloRequest) (*hello.HelloResponse, error) {
	return &hello.HelloResponse{Message: fmt.Sprintf("hello %s", req.Name)}, nil
}

// mockPolarisConsumer mock 北极星SDK Consumer
type mockPolarisConsumer struct {
	SDKOwner api.SDKOwner
}

// newMockPolarisConsumer 创建一个北极星SDK Consumer
func newMockPolarisConsumer() *mockPolarisConsumer {
	return &mockPolarisConsumer{}
}

// SDKContext 实现api.SDKOwner接口
func (mockPolarisConsumer) SDKContext() api.SDKContext {
	return nil
}

var (
	instance1 = polarispb.NewInstanceInProto(&namingpb.Instance{
		Host: &wrappers.StringValue{Value: "127.0.0.1"},
		Port: &wrappers.UInt32Value{Value: 8000}, // 联通的实例
	}, &model.ServiceKey{
		Namespace: "Production",
		Service:   "grpc.service",
	}, nil)
	instance2 = polarispb.NewInstanceInProto(&namingpb.Instance{
		Host: &wrappers.StringValue{Value: "127.0.0.1"},
		Port: &wrappers.UInt32Value{Value: 8001}, // 不通的实例
	}, &model.ServiceKey{
		Namespace: "Production",
		Service:   "grpc.service",
	}, nil)
	instance3 = polarispb.NewInstanceInProto(&namingpb.Instance{
		Host:    &wrappers.StringValue{Value: "127.0.0.1"},
		Port:    &wrappers.UInt32Value{Value: 8002},
		Isolate: &wrappers.BoolValue{Value: true}, // 通但是手动隔离的实例
	}, &model.ServiceKey{
		Namespace: "Production",
		Service:   "grpc.service",
	}, nil)
)

// GetOneInstance 实现ConsumerAPI接口
func (mockPolarisConsumer) GetOneInstance(req *api.GetOneInstanceRequest) (*model.InstancesResponse, error) {
	log.Printf("call GetOneInstance: req:%+v", req)
	ret := &model.InstancesResponse{
		Instances: []model.Instance{instance1},
	}
	return ret, nil
}

// GetInstances 实现ConsumerAPI接口
func (mockPolarisConsumer) GetInstances(req *api.GetInstancesRequest) (*model.InstancesResponse, error) {
	log.Printf("call GetInstances: req:%+v", req)
	ret := &model.InstancesResponse{
		Instances: []model.Instance{
			instance1, instance2,
		},
	}
	return ret, nil
}

// GetAllInstances 实现ConsumerAPI接口
func (mockPolarisConsumer) GetAllInstances(req *api.GetAllInstancesRequest) (*model.InstancesResponse, error) {
	log.Printf("call GetAllInstances: req:%+v", req)
	ret := &model.InstancesResponse{
		Instances: []model.Instance{
			instance1, instance2, instance3,
		},
	}
	return ret, nil
}

// GetRouteRule 实现ConsumerAPI接口
func (mockPolarisConsumer) GetRouteRule(req *api.GetServiceRuleRequest) (*model.ServiceRuleResponse, error) {
	return nil, nil
}

// UpdateServiceCallResult 实现ConsumerAPI接口
func (mockPolarisConsumer) UpdateServiceCallResult(req *api.ServiceCallResult) error {
	log.Printf("call UpdateServiceCallResult: req:%+v", req)
	return nil
}

// Destroy 实现ConsumerAPI接口
func (mockPolarisConsumer) Destroy() {}

// WatchService 实现ConsumerAPI接口
func (mockPolarisConsumer) WatchService(req *api.WatchServiceRequest) (*model.WatchServiceResponse, error) {
	return nil, nil
}

// GetMeshConfig 实现ConsumerAPI接口
func (mockPolarisConsumer) GetMeshConfig(req *api.GetMeshConfigRequest) (*model.MeshConfigResponse, error) {
	return nil, nil
}

// GetMesh 实现ConsumerAPI接口
func (mockPolarisConsumer) GetMesh(req *api.GetMeshRequest) (*model.MeshResponse, error) {
	return nil, nil
}

// GetServicesByBusiness 实现ConsumerAPI接口
func (mockPolarisConsumer) GetServicesByBusiness(req *api.GetServicesRequest) (*model.ServicesResponse, error) {
	return nil, nil
}

// InitCalleeService 实现ConsumerAPI接口
func (mockPolarisConsumer) InitCalleeService(req *api.InitCalleeServiceRequest) error {
	return nil
}

// mockPolarisConsumerFail mock 北极星SDK Consumer
type mockPolarisConsumerFail struct {
	SDKOwner api.SDKOwner
}

// newMockPolarisConsumerFail 创建一个北极星SDK Consumer
func newMockPolarisConsumerFail() *mockPolarisConsumerFail {
	return &mockPolarisConsumerFail{}
}

// SDKContext 实现api.SDKOwner接口
func (mockPolarisConsumerFail) SDKContext() api.SDKContext {
	return nil
}

// GetOneInstance 实现ConsumerAPI接口
func (mockPolarisConsumerFail) GetOneInstance(req *api.GetOneInstanceRequest) (*model.InstancesResponse, error) {
	log.Printf("call GetOneInstance: req:%+v", req)
	ret := &model.InstancesResponse{
		Instances: []model.Instance{},
	}
	return ret, nil
}

// GetInstances 实现ConsumerAPI接口
func (mockPolarisConsumerFail) GetInstances(req *api.GetInstancesRequest) (*model.InstancesResponse, error) {
	log.Printf("call GetInstances: req:%+v", req)
	ret := &model.InstancesResponse{
		Instances: []model.Instance{instance1, instance2, instance3},
	}
	return ret, nil
}

// GetAllInstances 实现ConsumerAPI接口
func (mockPolarisConsumerFail) GetAllInstances(req *api.GetAllInstancesRequest) (*model.InstancesResponse, error) {
	log.Printf("call GetAllInstances: req:%+v", req)
	ret := &model.InstancesResponse{
		Instances: []model.Instance{},
	}
	return ret, nil
}

// GetRouteRule 实现ConsumerAPI接口
func (mockPolarisConsumerFail) GetRouteRule(req *api.GetServiceRuleRequest) (*model.ServiceRuleResponse, error) {
	return nil, nil
}

// UpdateServiceCallResult 实现ConsumerAPI接口
func (mockPolarisConsumerFail) UpdateServiceCallResult(req *api.ServiceCallResult) error {
	log.Printf("call UpdateServiceCallResult: req:%+v", req)
	return nil
}

// Destroy 实现ConsumerAPI接口
func (mockPolarisConsumerFail) Destroy() {}

// WatchService 实现ConsumerAPI接口
func (mockPolarisConsumerFail) WatchService(req *api.WatchServiceRequest) (*model.WatchServiceResponse, error) {
	return nil, nil
}

// GetMeshConfig 实现ConsumerAPI接口
func (mockPolarisConsumerFail) GetMeshConfig(req *api.GetMeshConfigRequest) (*model.MeshConfigResponse, error) {
	return nil, nil
}

// GetMesh 实现ConsumerAPI接口
func (mockPolarisConsumerFail) GetMesh(req *api.GetMeshRequest) (*model.MeshResponse, error) {
	return nil, nil
}

// GetServicesByBusiness 实现ConsumerAPI接口
func (mockPolarisConsumerFail) GetServicesByBusiness(req *api.GetServicesRequest) (*model.ServicesResponse, error) {
	return nil, nil
}

// InitCalleeService 实现ConsumerAPI接口
func (mockPolarisConsumerFail) InitCalleeService(req *api.InitCalleeServiceRequest) error {
	return nil
}

func TestPolarisResolverFail(t *testing.T) {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	grpclog.SetLoggerV2(grpclog.NewLoggerV2WithVerbosity(os.Stdout, os.Stdout, os.Stderr, 999))
	polarisConsumer := newMockPolarisConsumerFail()
	Init(Conf{
		PolarisConsumer: polarisConsumer,
		SyncInterval:    time.Second * -1,
	})
	// init grpc server
	srv := grpc.NewServer()
	hello.RegisterHelloServer(srv, &Hello{})
	go func() {
		address := "0.0.0.0:8003"
		listen, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatalf("FailTest : failed to listen: %v", err)
		}
		err = srv.Serve(listen)
		if err != nil {
			log.Fatalf("FailTest : failed to Serv:%s", err)
		}
	}()
	go func() {
		address := "0.0.0.0:8005"
		listen, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatalf("FailTest : failed to listen: %v", err)
		}
		err = srv.Serve(listen)
		if err != nil {
			log.Fatalf("FailTest : failed to Serv:%s", err)
		}
	}()
	time.Sleep(time.Millisecond * 200)
	//  grpc client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := grpc.DialContext(ctx, "polaris://Production/grpc.service", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("failed to dial:%s", err)
	}
	rpcClient := hello.NewHelloClient(conn)
	for i := 0; i < 5; i++ {
		start := time.Now()
		resp, err := rpcClient.SayHello(ctx, &hello.HelloRequest{Name: "polaris"})
		if err != nil {
			t.Fatal(err)
		}
		log.Printf("FailTest : call cost:%v, resp:%v, err:%v", time.Since(start), resp, err)
		time.Sleep(time.Second)
	}
}
