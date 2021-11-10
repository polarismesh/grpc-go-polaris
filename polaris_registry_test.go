package grpcpolaris

import (
	"log"
	"testing"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
)

// mockPolarisProducerAPI mock 北极星SDK Limit
type mockPolarisProducerAPI struct {
	SDKOwner api.SDKOwner
}

// mockPolarisProducerAPI 创建一个北极星SDK Producer
func newMockPolarisProducerAPI() *mockPolarisProducerAPI {
	return &mockPolarisProducerAPI{}
}

// SDKContext 实现api.SDKOwner接口
func (mockPolarisProducerAPI) SDKContext() api.SDKContext {
	return nil
}

// GetOneInstance 实现ProducerAPI接口
func (mockPolarisProducerAPI) Register(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
	log.Printf("call Register: req:%+v", req)
	ret := &model.InstanceRegisterResponse{
		InstanceID: "test-inst",
		Existed:    false,
	}
	return ret, nil
}

// GetOneInstance 实现ProducerAPI接口
func (mockPolarisProducerAPI) Deregister(req *api.InstanceDeRegisterRequest) error {
	log.Printf("call Deregister: req:%+v", req)
	return nil
}

// GetOneInstance 实现ProducerAPI接口
func (mockPolarisProducerAPI) Heartbeat(req *api.InstanceHeartbeatRequest) error {
	log.Printf("call Heartbeat: req:%+v", req)
	return nil
}

// Destroy 实现ProducerAPI接口
func (mockPolarisProducerAPI) Destroy() {}

func newMockPolarisRegister() *PolarisRegister {
	return &PolarisRegister{
		Namespace:         "test-ns",
		Service:           "test-service",
		ServiceToken:      "test-token",
		Host:              "test-ip",
		Port:              8000,
		HeartbeatInterval: time.Duration(3 * time.Second),
		ProviderAPI:       newMockPolarisProducerAPI(),
	}
}

func TestRegisterAndHeartbeat(t *testing.T) {
	mockRegister := newMockPolarisRegister()
	mockRegister.RegisterAndHeartbeat()
}

func TestDeRegister(t *testing.T) {
	mockRegister := newMockPolarisRegister()
	mockRegister.DeRegister()
}
