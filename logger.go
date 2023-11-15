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
	"io"
	"sync/atomic"
)

type LogLevel int

const (
	_ LogLevel = iota
	LogDebug
	LogInfo
	LogWarn
	LogError
)

type Logger interface {
	SetWriter(io.WriteCloser)
	SetLevel()
	Debug(format string, args interface{})
	Info(format string, args interface{})
	Warn(format string, args interface{})
	Error(format string, args interface{})
}

type defaultLogger struct {
	writerRef atomic.Value
	levelRef  atomic.Value
}

func (l *defaultLogger) SetWriter(writer io.WriteCloser) {
	l.writerRef.Store(writer)
}

func (l *defaultLogger) SetLevel(level LogLevel) {
	l.levelRef.Store(level)
}

func (l *defaultLogger) Debug(format string, args interface{}) {

}

func (l *defaultLogger) Info(format string, args interface{}) {

}

func (l *defaultLogger) Warn(format string, args interface{}) {

}

func (l *defaultLogger) Error(format string, args interface{}) {

}
