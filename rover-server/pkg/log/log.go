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

var Log logr.Logger

const (
	jsonLogging    = "json"
	consoleLogging = "console"
)

func Init() {
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	zapLogLevel, err := zapcore.ParseLevel(viper.GetString("log.level"))
	if err != nil {
		zapLogLevel = zapcore.InfoLevel
	}

	encoding := viper.GetString("log.encoding")
	if encoding == consoleLogging {
		logCfg.Encoding = consoleLogging
	} else {
		encoding = jsonLogging
	}
	logCfg.Encoding = encoding
	logCfg.Level.SetLevel(zapLogLevel)
	zapLog := zap.Must(logCfg.Build())
	Log = zapr.NewLogger(zapLog)
	Log.Info("Logger initialized", "encoding", encoding, "level", zapLogLevel)
}
