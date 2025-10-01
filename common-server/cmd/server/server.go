// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/telekom/controlplane/common-server/internal/config"
	"github.com/telekom/controlplane/common-server/internal/crd"
	"github.com/telekom/controlplane/common-server/pkg/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/dynamic"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	logLevel    string
	kubeContext string
	configPath  string
)

func init() {
	flag.StringVar(&logLevel, "loglevel", "info", "Log level")
	flag.StringVar(&kubeContext, "kubecontext", "", "Kubeconfig context")
	flag.StringVar(&configPath, "configfile", "config/default.yaml", "Path to config file")
}

func main() {
	flag.Parse()

	logCfg := zap.NewProductionConfig()
	logCfg.DisableCaller = true
	logCfg.DisableStacktrace = true
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapLogLevel, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		zapLogLevel = zapcore.InfoLevel
	}

	logCfg.Level.SetLevel(zapLogLevel)
	zapLog := zap.Must(logCfg.Build())
	log := zapr.NewLogger(zapLog)
	ctx := server.SignalHandler(context.Background())
	ctx = logr.NewContext(ctx, log)

	cfg, err := kconfig.GetConfigWithContext(kubeContext)
	if err != nil {
		log.Error(err, "Failed to get kubeconfig")
		return
	}

	crd.InitCrdResolver(cfg)
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	serverCfg, err := config.ReadConfig(configPath)
	if err != nil {
		log.Error(err, "Failed to read config file")
		return
	}

	server, err := serverCfg.BuildServer(ctx, dynamicClient, log)
	if err != nil {
		log.Error(err, "Failed to build server")
		return
	}

	go func() {
		if err := server.Start(serverCfg.Address); err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	if err = server.App.Shutdown(); err != nil {
		log.Error(err, "Failed to shutdown server")
	}
}
