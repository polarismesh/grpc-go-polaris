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
	"runtime"
	"sync/atomic"
	"time"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LogLevel int

const (
	_ LogLevel = iota
	LogDebug
	LogInfo
	LogWarn
	LogError
)

func (l LogLevel) String() string {
	switch l {
	case LogDebug:
		return "[debug]"
	case LogInfo:
		return "[info]"
	case LogWarn:
		return "[warn]"
	case LogError:
		return "[error]"
	default:
		return ""
	}
}

var _log Logger = newDefaultLogger()

func SetLogger(logger Logger) {
	_log = logger
}

func GetLogger() Logger {
	return _log
}

type Logger interface {
	SetLevel(LogLevel)
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

type defaultLogger struct {
	writer   zapcore.Core
	levelRef atomic.Value
}

func newDefaultLogger() *defaultLogger {
	encoderCfg := zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		TimeKey:        "time",
		NameKey:        "name",
		CallerKey:      "caller",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   "./logs/grpc-go-polaris.log", // 文件位置
		MaxSize:    100,                          // 进行切割之前,日志文件的最大大小(MB为单位)
		MaxAge:     7,                            // 保留旧文件的最大天数
		MaxBackups: 100,                          // 保留旧文件的最大个数
		Compress:   true,                         // 是否压缩/归档旧文件
	})

	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderCfg), w, zap.InfoLevel)
	return &defaultLogger{
		writer: core,
	}
}

func (l *defaultLogger) SetLevel(level LogLevel) {
	l.levelRef.Store(level)
}

func (l *defaultLogger) Debug(format string, args ...interface{}) {
	l.printf(LogDebug, format, args...)
}

func (l *defaultLogger) Info(format string, args ...interface{}) {
	l.printf(LogInfo, format, args...)

}

func (l *defaultLogger) Warn(format string, args ...interface{}) {
	l.printf(LogWarn, format, args...)
}

func (l *defaultLogger) Error(format string, args ...interface{}) {
	l.printf(LogError, format, args...)
}

func (l *defaultLogger) printf(expectLevel LogLevel, format string, args ...interface{}) {
	zapL := func() zapcore.Level {
		switch expectLevel {
		case LogDebug:
			return zapcore.DebugLevel
		case LogInfo:
			return zapcore.InfoLevel
		case LogWarn:
			return zapcore.WarnLevel
		case LogError:
			return zapcore.ErrorLevel
		default:
			return zapcore.InfoLevel
		}
	}()

	if !l.writer.Enabled(zapL) {
		return
	}

	msg := fmt.Sprintf(format, args...)
	e := zapcore.Entry{
		Message: msg,
		Level:   zapL,
		Time:    time.Now(),
	}

	e.Caller = zapcore.NewEntryCaller(runtime.Caller(2))
	_ = l.writer.Write(e, nil)
}
