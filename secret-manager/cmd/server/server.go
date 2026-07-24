// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
	cs "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/secret-manager/cmd/server/config"
	"github.com/telekom/controlplane/secret-manager/internal/api"
	"github.com/telekom/controlplane/secret-manager/internal/handler"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache/metrics"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/conjur/bouncer"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/kubernetes"
	"github.com/telekom/controlplane/secret-manager/pkg/controller"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctrlr "sigs.k8s.io/controller-runtime"
)

const (
	trueStr = "true"
)

var (
	logLevel    string
	configFile  string
	backendType string
)

func init() {
	flag.StringVar(&logLevel, "loglevel", "info", "log level")
	flag.StringVar(&configFile, "configfile", "", "path to config file")
	flag.StringVar(&backendType, "backend", "", "backend type (kubernetes, conjur)")
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
		cfg.Backend.Type = "kubernetes"
	}

	shouldCache := cfg.Backend.GetDefault("disable_cache", "false") != trueStr
	if !shouldCache {
		log.V(1).Info("cache is disabled")
	}

	switch cfg.Backend.Type {
	case "conjur":
		bouncer.RegisterMetrics(prometheus.DefaultRegisterer)

		conjurWriteApi := conjur.NewConjurApiMetrics(conjur.NewWriteApiOrDie())
		conjurReadApi := conjur.NewConjurApiMetrics(conjur.NewReadOnlyApiOrDie())

		conjurBackend := conjur.NewBackend(conjurWriteApi, conjurReadApi)
		conjurBackend.WithBouncer(bouncer.NewLocker("secret-write"))

		if shouldCache {
			cacheDuration, err := time.ParseDuration(cfg.Backend.GetDefault("cache_duration", "10s"))
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse cache duration")
			}
			cacheMaxCostStr := cfg.Backend.GetDefault("cache_max_cost_mb", "100")
			cacheMaxCostMb, err := strconv.ParseInt(cacheMaxCostStr, 10, 0)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse cache max cost")
			}
			cacheMaxCostBytes := cacheMaxCostMb << 20 // convert MB to bytes
			log.V(1).Info("cache is enabled", "duration", cacheDuration.String(), "max_cost_mb", cacheMaxCostMb)

			cachedBackend := cache.NewCachedBackend(conjurBackend,
				cache.WithTTL(cacheDuration),
				cache.WithMaxCost(cacheMaxCostBytes),
			)
			metrics.RegisterMetrics(prometheus.DefaultRegisterer, cachedBackend.CacheSizeBytes)
			onboarder := conjur.NewOnboarder(conjurWriteApi, cachedBackend)
			onboarder.WithBouncer(bouncer.NewLocker("secret-onboard"))
			c = controller.NewController(cachedBackend, onboarder)
		} else {
			onboarder := conjur.NewOnboarder(conjurWriteApi, conjurBackend)
			onboarder.WithBouncer(bouncer.NewLocker("secret-onboard"))
			c = controller.NewController(conjurBackend, onboarder)
		}

	case "kubernetes":
		k8sClient, err := kubernetes.NewCachedClient(ctx, ctrlr.GetConfigOrDie())
		if err != nil {
			return nil, errors.Wrap(err, "failed to create kubernetes client")
		}
		backend := kubernetes.NewBackend(k8sClient)
		onboarder := kubernetes.NewOnboarder(k8sClient)
		c = controller.NewController(backend, onboarder)

	default:
		return nil, errors.Errorf("unknown backend type: %s", cfg.Backend.Type)
	}

	return c, nil
}

func main() {
	flag.Parse()
	log := setupLog(logLevel)

	ctx := cs.SignalHandler(context.Background())

	ctrlr.SetLogger(log)
	cfg := config.GetConfigOrDie(configFile)

	ctrl, err := newController(ctx, cfg)
	if err != nil {
		log.Error(err, "failed to create controller")
		return
	}

	appCfg := cs.NewAppConfig()
	appCfg.CtxLog = log
	appCfg.ErrorHandler = handler.ErrorHandler

	h := api.NewStrictHandler(handler.NewHandler(ctrl), nil)

	// Pure-k8s server: adminContext=false (handlers don't read BusinessContext);
	// internal=true yields K8sFamily with open access (empty accessConfig = any
	// authenticated in-cluster SA). A k8s block with no issuer auto-uses the
	// in-cluster issuer.
	lc := cfg.Listeners.Internal
	if lc == nil {
		log.Error(errors.New("no internal listener configured"), "secret-manager requires listeners.internal")
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
