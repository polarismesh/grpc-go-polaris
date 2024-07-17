/**
 * Tencent is pleased to support the open source community by making Polaris available.
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

package grpcpolaris

import (
	"strings"
	"sync"
	"time"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
)

const (
	lbConfig = `
{
    "loadBalancingConfig":[
        {
            "%s":{}
        }
    ]
}
`
)

var (
	// DefaultNamespace default namespace when namespace is not set
	DefaultNamespace = "default"
	// DefaultTTL default ttl value when ttl is not set
	DefaultTTL = 5
	// DefaultGracefulStopMaxWaitDuration default stop max wait duration when not set
	DefaultGracefulStopMaxWaitDuration = 30 * time.Second
	// DefaultDelayStopWaitDuration default delay time before stop
	DefaultDelayStopWaitDuration = 4 * 2 * time.Second
	// MinGracefulStopWaitDuration low bound of stop wait duration
	MinGracefulStopWaitDuration = 5 * time.Second
)

const (
	polarisCallerServiceKey   = "polaris.request.caller.service"
	polarisCallerNamespaceKey = "polaris.request.caller.namespace"
)

var (
	ctxRef              = 0
	polarisContext      api.SDKContext
	polarisConfig       config.Configuration
	mutexPolarisContext sync.Mutex
	oncePolarisConfig   sync.Once
)

func ClosePolarisContext() {
	mutexPolarisContext.Lock()
	defer mutexPolarisContext.Unlock()
	if nil == polarisContext {
		return
	}
	ctxRef--
	if ctxRef == 0 {
		polarisContext.Destroy()
		polarisContext = nil
	}
}

// PolarisContext get or init the global polaris context
func PolarisContext() (api.SDKContext, error) {
	mutexPolarisContext.Lock()
	defer mutexPolarisContext.Unlock()
	ctxRef++
	if nil != polarisContext {
		return polarisContext, nil
	}
	var err error
	polarisContext, err = api.InitContextByConfig(PolarisConfig())
	return polarisContext, err
}

func setPolarisContext(sdkContext api.SDKContext) {
	mutexPolarisContext.Lock()
	defer mutexPolarisContext.Unlock()
	polarisContext = sdkContext
}

// PolarisConfig get or init the global polaris configuration
func PolarisConfig() config.Configuration {
	if polarisConfig == nil {
		oncePolarisConfig.Do(func() {
			polarisConfig = api.NewConfiguration()
		})
	}
	return polarisConfig
}

// setPolarisConfig set the global polaris configuration
func setPolarisConfig(cfg config.Configuration) {
	polarisConfig = cfg
}

func extractBareMethodName(fullMethodName string) string {
	index := strings.LastIndex(fullMethodName, "/")
	if index == -1 {
		return ""
	}
	return fullMethodName[index+1:]
}

func extractBareServiceName(fullMethodName string) string {
	index := strings.LastIndex(fullMethodName, "/")
	if index == -1 {
		return ""
	}
	return fullMethodName[:index]
}
