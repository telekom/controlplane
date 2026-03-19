// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/telekom/controlplane/controlplane-api/ent"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/internal/database"
	gqlcontroller "github.com/telekom/controlplane/controlplane-api/internal/graphql"
	"github.com/telekom/controlplane/controlplane-api/internal/interceptor"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
)

func main() {
	log := setupLogger()
	ctx := logr.NewContext(context.Background(), log)
	ctx = cserver.SignalHandler(ctx)

	dbURL := envOrDefault("DATABASE_URL", "postgres://controlplane:controlplane@localhost:5432/controlplane?sslmode=disable")
	addr := envOrDefault("LISTEN_ADDR", ":8080")
	playgroundEnabled := envOrDefault("PLAYGROUND_ENABLED", "true") == "true"

	client, err := database.NewEntClient(ctx, dbURL)
	if err != nil {
		log.Error(err, "failed to create ent client")
		os.Exit(1)
	}
	client.Intercept(interceptor.TeamFilterInterceptor())

	srv := newGraphQLServer(client)

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log
	s := cserver.NewServerWithApp(cserver.NewAppWithConfig(appCfg))

	probesCtrl := cserver.NewProbesController()
	probesCtrl.Register(s.App, cserver.ControllerOpts{})

	gqlCtrl := gqlcontroller.NewController(srv, playgroundEnabled)
	s.RegisterController(gqlCtrl, cserver.ControllerOpts{
		Prefix:         "/graphql",
		AllowedMethods: []string{"HEAD", "GET", "POST", "OPTIONS"},
		Security: security.SecurityOpts{
			Enabled: envOrDefault("SECURITY_ENABLED", "false") == "true",
			Log:     log.WithName("security"),
		},
	})

	go func() {
		if err := s.Start(addr); err != nil {
			log.Error(err, "failed to start server")
		}
	}()
	log.Info("server started", "addr", addr)

	<-ctx.Done()
	log.Info("shutting down server")

	if err := s.App.Shutdown(); err != nil {
		log.Error(err, "failed to gracefully shutdown server")
	}

	if err := client.Close(); err != nil {
		log.Error(err, "failed to close database client")
	}
}

func setupLogger() logr.Logger {
	logCfg := zap.NewProductionConfig()
	logCfg.DisableStacktrace = true
	logCfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	zapLogLevel, err := zapcore.ParseLevel(envOrDefault("LOG_LEVEL", "info"))
	if err != nil {
		zapLogLevel = zapcore.InfoLevel
	}
	logCfg.Level.SetLevel(zapLogLevel)
	return zapr.NewLogger(zap.Must(logCfg.Build()))
}

func newGraphQLServer(client *ent.Client) *handler.Server {
	srv := handler.New(resolvers.NewSchema(client))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	srv.AroundOperations(gqlcontroller.ViewerFromBusinessContext(client))

	return srv
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
