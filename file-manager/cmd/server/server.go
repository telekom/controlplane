// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"os"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrlr "sigs.k8s.io/controller-runtime"

	cs "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/file-manager/cmd/server/config"
	"github.com/telekom/controlplane/file-manager/internal/api"
	"github.com/telekom/controlplane/file-manager/internal/handler"
	"github.com/telekom/controlplane/file-manager/pkg/backend/buckets"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
)

var (
	logLevel    string
	configFile  string
	backendType string
)

func init() {
	flag.StringVar(&logLevel, "loglevel", "info", "log level")
	flag.StringVar(&configFile, "configfile", "", "path to config file")
	flag.StringVar(&backendType, "backend", "", "backend type (buckets)")
}

func setupLog(logLevel string) logr.Logger {
	logCfg := zap.NewProductionConfig()
	logCfg.DisableCaller = true
	logCfg.DisableStacktrace = true
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logCfg.EncoderConfig.TimeKey = "time"
	zapLogLevel, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		zapLogLevel = zapcore.InfoLevel
	}

	logCfg.Level.SetLevel(zapLogLevel)
	zapLog := zap.Must(logCfg.Build())
	return zapr.NewLogger(zapLog)
}

func newController(ctx context.Context, cfg *config.ServerConfig) (c controller.Controller, err error) {
	log := logr.FromContextOrDiscard(ctx)
	if backendType != "" {
		cfg.Backend.Type = backendType
	}
	if cfg.Backend.Type == "" {
		cfg.Backend.Type = "buckets"
	}
	log.Info("Initializing backend", "type", cfg.Backend.Type)

	switch cfg.Backend.Type {
	case "buckets":
		var options []buckets.ConfigOption
		options = append(options, buckets.WithLogger(log))

		if endpoint := cfg.Backend.Get("endpoint"); endpoint != "" {
			options = append(options, buckets.WithEndpoint(endpoint))
			log.Info("Using bucket endpoint", "endpoint", endpoint)
		}

		if bucketName := cfg.Backend.Get("bucket_name"); bucketName != "" {
			options = append(options, buckets.WithBucketName(bucketName))
			log.Info("Using bucket", "bucket_name", bucketName)
		}

		if insecure, err := strconv.ParseBool(cfg.Backend.Get("insecure_skip_tls")); err == nil && insecure {
			options = append(options, buckets.WithInsecureSkipTLS(true))
			log.Info("TLS verification is disabled")
		}

		// Create credentials
		creds, err := buckets.NewCredentials(buckets.AutoDiscoverProvider(cfg.Backend), buckets.WithProperties(cfg.Backend))
		if err != nil {
			log.Error(err, "Failed to initialize credentials")
			return nil, errors.Wrap(err, "failed to initialize credentials")
		}
		options = append(options, buckets.WithCredentials(creds))

		bucketConfig, err := buckets.NewBucketConfig(options...)
		if err != nil {
			log.Error(err, "Failed to initialize bucket configuration")
			return nil, errors.Wrap(err, "failed to initialize bucket configuration")
		}
		log.Info("Bucket configuration initialized successfully")

		// Create file uploader, downloader and deleter with the shared config
		fileDownloader := buckets.NewBucketFileDownloader(bucketConfig)
		fileUploader := buckets.NewBucketFileUploader(bucketConfig)
		fileDeleter := buckets.NewBucketFileDeleter(bucketConfig)

		c = controller.NewController(fileDownloader, fileUploader, fileDeleter)

	default:
		return nil, errors.Errorf("unknown backend type: %s", cfg.Backend.Type)
	}

	return c, nil
}

func main() {
	flag.Parse()
	log := setupLog(logLevel)

	ctx := cs.SignalHandler(context.Background())
	// Add logger to context early so it can be used by newController
	ctx = logr.NewContext(ctx, log.WithName("initialize server"))

	ctrlr.SetLogger(log)
	log.Info("Loading configuration file", "path", configFile)
	cfg := config.GetConfigOrDie(configFile)
	log.Info("Configuration loaded successfully")

	ctrl, err := newController(ctx, cfg)
	if err != nil {
		log.Error(err, "failed to create controller")
		return
	}

	appCfg := cs.NewAppConfig()
	appCfg.BodyLimit = 10 * 1024 * 1024 // 10 MB
	appCfg.CtxLog = log
	appCfg.ErrorHandler = handler.ErrorHandler

	h := api.NewStrictHandler(handler.NewHandler(ctrl), nil)

	// Pure-k8s server: adminContext=false (handlers don't read BusinessContext);
	// internal=true yields K8sFamily with open access (empty accessConfig = any
	// authenticated in-cluster SA). A k8s block with no issuer auto-uses the
	// in-cluster issuer.
	lc := cfg.Listeners.Internal
	if lc == nil {
		log.Error(errors.New("no internal listener configured"), "file-manager requires listeners.internal")
		os.Exit(1)
	}
	family, err := cs.FamilyFromListenerConfig(*lc, nil, cs.WithInternal())
	if err != nil {
		log.Error(err, "failed to build security family for internal listener", "address", lc.Address)
		os.Exit(1)
	}

	ms := &cs.MultiServer{
		AppConfig: appCfg,
		TLS:       cfg.TLS.ToServerTLS(),
		Listeners: cs.Listeners{
			Internal: &cs.Listener{Address: lc.Address, Family: family},
		},
		Register: func(router fiber.Router, guard fiber.Handler) {
			apiGroup := router.Group("/api")
			api.RegisterHandlersWithOptions(apiGroup, h, api.FiberServerOptions{})
		},
	}

	ctx = logr.NewContext(ctx, log.WithName("server"))
	if err := ms.Run(ctx); err != nil {
		log.Error(err, "server exited with error")
		os.Exit(1)
	}
	log.Info("shutting down server...")
}
