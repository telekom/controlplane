// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	cs "github.com/telekom/controlplane/common-server/pkg/server"
	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
	"github.com/telekom/controlplane/common-server/pkg/server/serve"
	"github.com/telekom/controlplane/common-server/pkg/util"
	"github.com/telekom/controlplane/file-manager/cmd/server/config"
	"github.com/telekom/controlplane/file-manager/internal/api"
	"github.com/telekom/controlplane/file-manager/internal/handler"
	"github.com/telekom/controlplane/file-manager/internal/middleware"
	"github.com/telekom/controlplane/file-manager/pkg/backend/buckets"
	"github.com/telekom/controlplane/file-manager/pkg/controller"
	"go.uber.org/zap/zapcore"
	ctrlr "sigs.k8s.io/controller-runtime"
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

//nolint:unparam
func newController(ctx context.Context, cfg *config.ServerConfig, log logr.Logger) (c controller.Controller, err error) {
	if backendType != "" {
		cfg.Backend.Type = backendType
	}
	if cfg.Backend.Type == "" {
		cfg.Backend.Type = "buckets"
	}
	log.Info("Initializing backend", "type", cfg.Backend.Type)

	switch cfg.Backend.Type {
	case "buckets":
		// Create S3 config options from the server config
		var options []buckets.ConfigOption

		if endpoint := cfg.Backend.Get("endpoint"); endpoint != "" {
			options = append(options, buckets.WithEndpoint(endpoint))
			log.Info("Using bucket endpoint", "endpoint", endpoint)
		}

		if stsEndpoint := cfg.Backend.Get("sts_endpoint"); stsEndpoint != "" {
			options = append(options, buckets.WithSTSEndpoint(stsEndpoint))
			log.Info("Using STS endpoint", "sts_endpoint", stsEndpoint)
		}

		if bucketName := cfg.Backend.Get("bucket_name"); bucketName != "" {
			options = append(options, buckets.WithBucketName(bucketName))
			log.Info("Using bucket", "bucket_name", bucketName)
		}

		if roleArn := cfg.Backend.Get("role_arn"); roleArn != "" {
			options = append(options, buckets.WithRoleSessionArn(roleArn))
			log.Info("Using role ARN", "role_arn", roleArn)
		}

		if tokenPath := cfg.Backend.Get("token_path"); tokenPath != "" {
			options = append(options, buckets.WithTokenPath(tokenPath))
			log.Info("Using token path", "token_path", tokenPath)
		}

		// Check if environment variable is set
		if webToken := os.Getenv("MC_WEB_IDENTITY_TOKEN"); webToken != "" {
			log.V(1).Info("Found MC_WEB_IDENTITY_TOKEN environment variable")
		} else {
			log.V(1).Info("MC_WEB_IDENTITY_TOKEN environment variable not set")
		}

		// Initialize bucket configuration with context
		log.Info("Initializing bucket configuration")
		bucketConfig, err := buckets.NewBucketConfigWithLogger(log, options...)
		if err != nil {
			log.Error(err, "Failed to initialize bucket configuration")
			return nil, errors.Wrap(err, "failed to initialize bucket configuration")
		}
		log.Info("Bucket configuration initialized successfully")

		// Create file uploader and downloader with the shared config
		fileDownloader := buckets.NewBucketFileDownloader(bucketConfig)
		fileUploader := buckets.NewBucketFileUploader(bucketConfig)

		c = controller.NewController(fileDownloader, fileUploader)

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
	ctx = logr.NewContext(ctx, log)

	ctrlr.SetLogger(log)
	log.Info("Loading configuration file", "path", configFile)
	cfg := config.GetConfigOrDie(configFile)
	log.Info("Configuration loaded successfully")

	ctrl, err := newController(ctx, cfg, log)
	if err != nil {
		log.Error(err, "failed to create controller")
		return
	}

	appCfg := cs.NewAppConfig()
	appCfg.CtxLog = &log
	appCfg.ErrorHandler = handler.ErrorHandler
	app := cs.NewAppWithConfig(appCfg)

	probesCtrl := cs.NewProbesController()
	probesCtrl.Register(app, cs.ControllerOpts{})

	apiGroup := app.Group("/api")
	handler := api.NewStrictHandler(handler.NewHandler(ctrl), nil)

	// Add bearer auth middleware to extract the token from the request
	log.Info("Registering bearer token middleware")
	apiGroup.Use(middleware.BearerAuthMiddleware(log))

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

		// Add server name to the logger that was already set in the context
		ctxLog := logr.FromContextOrDiscard(ctx)
		ctx = logr.NewContext(ctx, ctxLog.WithName("server"))
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
