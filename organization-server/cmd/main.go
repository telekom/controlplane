// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

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
	cpapiClient := client.NewCPAPIClient(cfg.CPAPIEndpoint, tokenSource)
	roverClient := client.NewRoverClient(cfg.RoverEndpoint)

	// Fiber app.
	app := fiber.New(fiber.Config{
		ReadTimeout:           5 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           60 * time.Second,
		DisableStartupMessage: true,
		JSONEncoder:           sonic.Marshal,
		JSONDecoder:           sonic.Unmarshal,
	})

	app.Use(recover.New(recover.Config{EnableStackTrace: true}))

	// Health/readiness probes (unauthenticated).
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	app.Get("/readyz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	// API routes under /organization/v1 — requires consumer token.
	api := app.Group("/organization/v1", mw.TokenDecode(log))

	// Register all endpoint handlers.
	h := handler.New(cpapiClient, roverClient, log)
	h.RegisterRoutes(api)

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = shutdownCtx // Fiber's Shutdown doesn't take a context
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
