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

package test

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"testing"

	"gopkg.in/check.v1"
)

// Test 测试用例主入口
func Test(t *testing.T) {
	go func() {
		log.Println(http.ListenAndServe("LOCALHOST:6060", nil))
	}()
	check.TestingT(t)
}

// 初始化测试套
func init() {
	log.Printf("start to test serverTestingSuite")
	check.Suite(&serverTestingSuite{})
	log.Printf("start to test clientTestingSuite")
	check.Suite(&clientTestingSuite{})
}
