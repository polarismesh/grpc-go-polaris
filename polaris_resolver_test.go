package grpcpolaris

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/resolver"
)

// TestParseTarget
func TestParseTarget(t *testing.T) {
	cases := []struct {
		caseName     string
		target       resolver.Target
		namespace    string
		service      string
		targetString string
		err          error
	}{
		{
			caseName: "v1Target",
			target: resolver.Target{
				Scheme:    Name,
				Authority: "Production",
				Endpoint:  "grpc.service",
			},
			namespace:    "Production",
			service:      "grpc.service",
			targetString: "polaris://Production/grpc.service"},
		{
			caseName: "v1Target-invalid",
			target: resolver.Target{
				Scheme:    Name,
				Authority: "",
				Endpoint:  "grpc.service",
			},
			namespace:    "Production",
			service:      "grpc.service",
			targetString: "polaris:///grpc.service",
			err:          errAddrMisMatch},
		{
			caseName: "v2Target",
			target: resolver.Target{
				Scheme:    Name,
				Authority: "",
				Endpoint:  "grpc.service?namespace=Production",
			},
			namespace:    "Production",
			service:      "grpc.service",
			targetString: "polaris:///grpc.service?namespace=Production"},
		{
			caseName: "v2Target-invalid",
			target: resolver.Target{
				Scheme:    Name,
				Authority: "",
				Endpoint:  "grpc.service?namespace=Productionx",
			},
			namespace:    "Production",
			service:      "grpc.service",
			targetString: "polaris:///grpc.service?namespace=Productionx",
			err:          errAddrMisMatch,
		},
	}
	for _, c := range cases {
		note := fmt.Sprintf("caseName:%s, targetString:%s", c.caseName, c.targetString)
		namespace, service, err := parseTarget(c.target)
		if c.err != nil {
			assert.Equal(t, c.err, errors.Unwrap(err), note)
			continue
		} else {
			assert.Equal(t, nil, nil, note)
		}
		assert.Equal(t, c.namespace, namespace, note)
		assert.Equal(t, c.service, service, note)
	}
}
