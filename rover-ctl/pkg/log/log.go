// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package log

import (
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	global logr.Logger
)

func SetGlobalLogger(logger logr.Logger) {
	global = logger
}
func L() logr.Logger {
	if global.IsZero() {
		global = NewLogger()
	}
	return global
}

// NewLogger creates a new logger with settings from viper config
func NewLogger() logr.Logger {
	logLevel := viper.GetString("log.level")
	zapLogLevel, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		zapLogLevel = zapcore.InfoLevel
	}

	logCfg := zap.NewProductionConfig()
	logCfg.Level = zap.NewAtomicLevelAt(zapLogLevel)
	logFormat := viper.GetString("log.format")
	switch logFormat {
	case "json":
		logCfg.Encoding = "json"
	case "console":
		logCfg.Encoding = "console"
	default:
		logCfg.Encoding = "console" // Default to console if unknown format
	}

	logCfg.DisableCaller = true
	logCfg.DisableStacktrace = true

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	encoderCfg.TimeKey = "timestamp"
	logCfg.EncoderConfig = encoderCfg

	zapLog := zap.Must(logCfg.Build())
	return zapr.NewLogger(zapLog)
}
