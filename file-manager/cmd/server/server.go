// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrlr "sigs.k8s.io/controller-runtime"

	cs "github.com/telekom/controlplane/common-server/pkg/server"
	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
	"github.com/telekom/controlplane/common-server/pkg/server/serve"
	"github.com/telekom/controlplane/common-server/pkg/util"
	"github.com/telekom/controlplane/file-manager/cmd/server/config"
	"github.com/telekom/controlplane/file-manager/internal/api"
	"github.com/telekom/controlplane/file-manager/internal/handler"
	"github.com/telekom/controlplane/file-manager/pkg/backend/buckets"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
)

var (
	logLevel    string
	disableTls  bool
	tlsCert     string
	tlsKey      string
	address     string
	configFile  string
	backendType string
)

func init() {
	flag.StringVar(&logLevel, "loglevel", "info", "log level")
	flag.BoolVar(&disableTls, "disable-tls", false, "disable TLS")
	flag.StringVar(&tlsCert, "tls-cert", "/etc/tls/tls.crt", "path to TLS certificate")
	flag.StringVar(&tlsKey, "tls-key", "/etc/tls/tls.key", "path to TLS key")
	flag.StringVar(&address, "address", ":8443", "server address")
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

		// Create credentials
		creds, _ := buckets.NewCredentials(buckets.AutoDiscoverProvider(cfg.Backend), buckets.WithProperties(cfg.Backend))
		options = append(options, buckets.WithCredentials(creds))

		bucketConfig, err := buckets.NewBucketConfig(options...)
		if err != nil {
			log.Error(err, "Failed to initialize bucket configuration")
			return nil, errors.Wrap(err, "failed to initialize bucket configuration")
		}
		log.Info("Bucket configuration initialized successfully")

		// Create file uploader and downloader with the shared config
		fileDownloader := buckets.NewBucketFileDownloader(bucketConfig)
		fileUploader := buckets.NewBucketFileUploader(bucketConfig)

		// Create file deleter and wire into controller
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
	appCfg.CtxLog = &log
	appCfg.ErrorHandler = handler.ErrorHandler
	app := cs.NewAppWithConfig(appCfg)

	probesCtrl := cs.NewProbesController()
	probesCtrl.Register(app, cs.ControllerOpts{})

	apiGroup := app.Group("/api")
	handler := api.NewStrictHandler(handler.NewHandler(ctrl), nil)

	if cfg.Security.Enabled {
		opts := []k8s.KubernetesAuthOption{
			k8s.WithTrustedIssuers(cfg.Security.TrustedIssuers...),
			k8s.WithJWKSetURLs(cfg.Security.JWKSetURLs...),
			k8s.WithAccessConfig(cfg.Security.AccessConfig...),
		}
		if util.IsRunningInCluster() {
			log.Info("🔑 Running in cluster")
			opts = append(opts, k8s.WithInClusterIssuer())
		}
		apiGroup.Use(k8s.NewKubernetesAuthz(opts...))
	}

	api.RegisterHandlersWithOptions(apiGroup, handler, api.FiberServerOptions{})

	go func() {
		if disableTls {
			fmt.Println("⚠️\tUsing HTTP instead of HTTPS. This is not secure.")
			if err := app.Listen(address); err != nil {
				log.Error(err, "failed to start server")
				os.Exit(1)
			}
			return
		}

		ctx = logr.NewContext(ctx, log.WithName("server"))
		if err := serve.ServeTLS(ctx, app, address, tlsCert, tlsKey); err != nil {
			log.Error(err, "failed to start server")
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down server...")
	if err := app.Shutdown(); err != nil {
		log.Error(err, "failed to shutdown server")
	}
}
