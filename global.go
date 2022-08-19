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
	"fmt"
	"log"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/config"
)

var (
	// DefaultNamespace default namespace when namespace is not set
	DefaultNamespace = "default"
	// DefaultTTL default ttl value when ttl is not set
	DefaultTTL = 20
	// LoadBalanceConfig config to do the balance
	LoadBalanceConfig = fmt.Sprintf("{\n  \"loadBalancingConfig\": [ { \"%s\": {} } ]}", scheme)
	// DefaultGracefulStopMaxWaitDuration default stop max wait duration when not set
	DefaultGracefulStopMaxWaitDuration = 30 * time.Second
	// DefaultDelayStopWaitDuration default delay time before stop
	DefaultDelayStopWaitDuration = 4 * 2 * time.Second
	// MinGracefulStopWaitDuration low bound of stop wait duration
	MinGracefulStopWaitDuration = 5 * time.Second
)

var (
	polarisContext      api.SDKContext
	polarisConfig       config.Configuration
	mutexPolarisContext sync.Mutex
	oncePolarisConfig   sync.Once
)

// PolarisContext get or init the global polaris context
func PolarisContext() (api.SDKContext, error) {
	mutexPolarisContext.Lock()
	defer mutexPolarisContext.Unlock()
	if nil != polarisContext {
		return polarisContext, nil
	}
	var err error
	polarisContext, err = api.InitContextByConfig(PolarisConfig())
	return polarisContext, err
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

// SetPolarisConfig set the global polaris configuration
func SetPolarisConfig(cfg config.Configuration) {
	bs, _ := yaml.Marshal(cfg)
	var err error
	polarisConfig, err = config.LoadConfiguration(bs)
	if err != nil {
		log.Printf("load config err (%+v)", err)
	}
}
