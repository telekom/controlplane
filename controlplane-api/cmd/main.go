// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	"github.com/telekom/controlplane/controlplane-api/ent"
	_ "github.com/telekom/controlplane/controlplane-api/ent/runtime"
	"github.com/telekom/controlplane/controlplane-api/internal/database"
	gqlcontroller "github.com/telekom/controlplane/controlplane-api/internal/graphql"
	"github.com/telekom/controlplane/controlplane-api/internal/interceptor"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
)

func main() {
	zapLog, _ := zap.NewProduction()
	log := zapr.NewLogger(zapLog)
	ctx := logr.NewContext(context.Background(), log)

	dbURL := envOrDefault("DATABASE_URL", "postgres://controlplane:controlplane@localhost:5432/controlplane?sslmode=disable")
	addr := envOrDefault("LISTEN_ADDR", ":8080")
	playgroundEnabled := envOrDefault("PLAYGROUND_ENABLED", "true") == "true"

	client, err := database.NewEntClient(ctx, dbURL)
	if err != nil {
		log.Error(err, "failed to create ent client")
		os.Exit(1)
	}
	defer client.Close()

	client.Intercept(interceptor.TeamFilterInterceptor())

	srv := newGraphQLServer(client)

	s := cserver.NewServer()

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := s.Start(addr); err != nil && err != http.ErrServerClosed {
			log.Error(err, "failed to start server")
		}
	}()
	log.Info("server started", "addr", addr)

	sig := <-quit
	log.Info("shutting down server", "signal", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.App.ShutdownWithContext(shutdownCtx); err != nil {
		log.Error(err, "failed to gracefully shutdown server")
	}
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
