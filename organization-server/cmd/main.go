// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	cserver "github.com/telekom/controlplane/common-server/pkg/server"

	"github.com/telekom/controlplane/organization-server/internal/client"
	"github.com/telekom/controlplane/organization-server/internal/config"
	"github.com/telekom/controlplane/organization-server/internal/handler"
	mw "github.com/telekom/controlplane/organization-server/internal/middleware"
)

func main() {
	cfg := config.Load()
	log := initLogger(cfg.LogLevel)

	log.Info("Starting organization-server",
		"port", cfg.Port,
		"cpapiEndpoint", cfg.CPAPIEndpoint,
		"roverEndpoint", cfg.RoverEndpoint,
	)

	// OAuth token source for admin authentication to CP API.
	var tokenSource *client.TokenSource
	if cfg.OAuthTokenURL != "" && cfg.OAuthClientID != "" {
		tokenSource = client.NewTokenSource(cfg.OAuthTokenURL, cfg.OAuthClientID, cfg.OAuthClientSecret)
		log.Info("OAuth token source configured")
	} else {
		log.Info("OAuth token source not configured, CP API calls will be unauthenticated")
	}

	// Upstream clients.
	cpapiClient := client.NewCPAPIClient(cfg.CPAPIEndpoint, tokenSource, cfg.CPAPICaFilePath)
	roverClient := client.NewRoverClient(cfg.RoverEndpoint, cfg.RoverScopePrefix)

	// Fiber app — use common-server for standard middleware (logging, metrics,
	// timeout, recover). Security is handled separately below because the facade
	// has a custom auth pipeline (JWT + IdentityExtraction + TeamAuthorization).
	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log
	app := cserver.NewAppWithConfig(appCfg)

	// Health/readiness probes (standard pattern from common-server).
	probes := cserver.NewProbesController()
	probes.Register(app, cserver.ControllerOpts{})

	// API routes under /organization/v1 — custom secured pipeline.
	api := app.Group("/organization/v1",
		mw.JWTValidation(log, cfg.TrustedIssuers),
		mw.IdentityExtraction(log, cfg.RoverEnvironment),
		mw.Obfuscate(),
	)

	// Register all endpoint handlers (TeamAuthorization applied per-route inside).
	teamAuth := mw.TeamAuthorization(log)
	h := handler.New(cpapiClient, roverClient, log)
	h.RegisterRoutes(api, teamAuth)

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Error(err, "Server failed")
			os.Exit(1)
		}
	}()

	log.Info("Server started", "address", ":"+cfg.Port)

	<-ctx.Done()
	log.Info("Shutting down...")

	if err := app.Shutdown(); err != nil {
		log.Error(err, "Shutdown error")
	}
	log.Info("Server stopped")
}

func initLogger(level string) logr.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(zapLevel)
	zapLog, err := zapCfg.Build()
	if err != nil {
		panic(err)
	}
	return zapr.NewLogger(zapLog)
}
