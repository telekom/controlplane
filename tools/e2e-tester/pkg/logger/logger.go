// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package logger provides a simple wrapper around zap global logger
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log levels
const (
	DebugLevel = "debug"
	InfoLevel  = "info"
	WarnLevel  = "warn"
	ErrorLevel = "error"
)

// Initialize sets up the global zap logger
func Initialize(logLevel string, development bool) error {
	// Parse log level
	var level zapcore.Level
	switch logLevel {
	case DebugLevel:
		level = zapcore.DebugLevel
	case InfoLevel:
		level = zapcore.InfoLevel
	case WarnLevel:
		level = zapcore.WarnLevel
	case ErrorLevel:
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// Build logger configuration
	zapCfg := zap.Config{
		Level:             zap.NewAtomicLevelAt(level),
		Development:       development,
		DisableCaller:     !development,
		DisableStacktrace: !development,
		Encoding:          "console",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	// Build the logger and replace global
	logger, err := zapCfg.Build()
	if err != nil {
		return err
	}

	// Replace global zap logger
	zap.ReplaceGlobals(logger)
	return nil
}

// Sync flushes any buffered log entries
func Sync() {
	_ = zap.L().Sync()
}
