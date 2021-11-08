package grpcpolaris

import (
	"context"
	"errors"
	"log"
	"testing"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/clock"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// mockPolarisLimitAPI mock 北极星SDK Limit
type mockPolarisLimitAPI struct {
	SDKOwner api.SDKOwner
}

// mockPolarisLimiter 创建一个北极星SDK Limit
func newMockPolarisLimitAPI() *mockPolarisLimitAPI {
	return &mockPolarisLimitAPI{}
}

// SDKContext 实现api.SDKOwner接口
func (mockPolarisLimitAPI) SDKContext() api.SDKContext {
	return nil
}

// GetOneInstance 实现ConsumerAPI接口
func (mockPolarisLimitAPI) GetQuota(req api.QuotaRequest) (api.QuotaFuture, error) {
	log.Printf("call GetQuota: req:%+v", req)
	mRequest := req.(*model.QuotaRequestImpl)
	if mRequest.GetService() == "pass" {
		//没有限流规则，直接放通
		resp := &model.QuotaResponse{
			Code: model.QuotaResultOk,
		}
		return model.NewQuotaFuture(resp, clock.GetClock().Now(), nil), nil
	}

	resp := &model.QuotaResponse{
		Code: model.QuotaResultLimited,
	}
	return model.NewQuotaFuture(resp, clock.GetClock().Now(), nil), nil
}

// Destroy 实现LimitAPI接口
func (mockPolarisLimitAPI) Destroy() {}

func newMockPassPolarisLimiter() *PolarisLimiter {
	return &PolarisLimiter{
		Namespace: "test-ns",
		Service:   "pass",
		Labels:    nil,
		LimitAPI:  newMockPolarisLimitAPI(),
	}
}

func newMockFailPolarisLimiter() *PolarisLimiter {
	return &PolarisLimiter{
		Namespace: "test-ns",
		Service:   "fail",
		Labels:    nil,
		LimitAPI:  newMockPolarisLimitAPI(),
	}
}

func TestUnaryServerInterceptorPass(t *testing.T) {
	interceptor := UnaryServerInterceptor(newMockPassPolarisLimiter())
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("test error")
	}
	info := &grpc.UnaryServerInfo{
		FullMethod: "TestMethod",
	}
	resp, err := interceptor(nil, nil, info, handler)
	assert.Nil(t, resp)
	assert.EqualError(t, err, "test error")
}

func TestUnaryServerInterceptorFail(t *testing.T) {
	interceptor := UnaryServerInterceptor(newMockFailPolarisLimiter())
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("test error")
	}
	info := &grpc.UnaryServerInfo{
		FullMethod: "TestMethod",
	}
	resp, err := interceptor(nil, nil, info, handler)
	assert.Nil(t, resp)
	assert.EqualError(t, err, "rpc error: "+
		"code = ResourceExhausted desc = TestMethod is rejected by polaris rate limiter, please retry later.")
}

func TestStreamServerInterceptorPass(t *testing.T) {
	interceptor := StreamServerInterceptor(newMockPassPolarisLimiter())
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return errors.New("test error")
	}
	info := &grpc.StreamServerInfo{
		FullMethod: "TestMethod",
	}
	err := interceptor(nil, nil, info, handler)
	assert.EqualError(t, err, "test error")
}

func TestStreamServerInterceptorFail(t *testing.T) {
	interceptor := StreamServerInterceptor(newMockFailPolarisLimiter())
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		return errors.New("test error")
	}
	info := &grpc.StreamServerInfo{
		FullMethod: "TestMethod",
	}
	err := interceptor(nil, nil, info, handler)
	assert.EqualError(t, err, "rpc error: "+
		"code = ResourceExhausted desc = TestMethod is rejected by polaris rate limiter, please retry later.")
}
