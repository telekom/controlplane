// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/telekom/controlplane/rover-server/internal/config"
	"github.com/telekom/controlplane/rover-server/internal/controller"
	"github.com/telekom/controlplane/rover-server/internal/oaslint"
	"github.com/telekom/controlplane/rover-server/internal/server"
	"github.com/telekom/controlplane/rover-server/pkg/log"
	"github.com/telekom/controlplane/rover-server/pkg/store"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(errors.Wrap(err, "failed to load configuration"))
	}

	log.Init()
	rootCtx := logr.NewContext(context.Background(), log.Log)

	stores := store.NewStores(rootCtx, kconfig.GetConfigOrDie())

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log.Log
	app := cserver.NewAppWithConfig(appCfg)

	probesCtrl := cserver.NewProbesController()
	probesCtrl.Register(app, cserver.ControllerOpts{})

	file.AppendOption(
		filesapi.WithSkipTLSVerify(cfg.FileManager.SkipTLS),
	)

	var linter oaslint.Linter
	if cfg.OasLinting.URL != "" {
		log.Log.Info("OAS linting enabled", "url", cfg.OasLinting.URL)
		linter = oaslint.NewExternalLinter(cfg.OasLinting.URL)
	}

	whitelistedBasepaths := parseDelimitedSet(cfg.OasLinting.WhitelistedBasepaths, ";")
	whitelistedCategories := parseDelimitedSet(cfg.OasLinting.WhitelistedCategories, ";")

	s := server.Server{
		Config:              cfg,
		Log:                 log.Log,
		ApiSpecifications:   controller.NewApiSpecificationController(stores, linter, whitelistedBasepaths, whitelistedCategories, cfg.OasLinting.ErrorMessage),
		Rovers:              controller.NewRoverController(stores),
		EventSpecifications: controller.NewEventSpecificationController(stores),
	}

	s.RegisterRoutes(app)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		err := app.Listen(cfg.Address)
		if err != nil && err != http.ErrServerClosed {
			log.Log.Error(err, "Failed to start server")
		}
	}()

	sig := <-quit
	log.Log.Info("Shutting down server", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Log.Error(err, "Failed to gracefully shutdown server")
	}

	log.Log.Info("Server gracefully stopped")
}

// parseDelimitedSet parses a delimited string into a set.
// Empty entries are ignored. Categories are lowercased for case-insensitive matching.
func parseDelimitedSet(s, delimiter string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, v := range strings.Split(s, delimiter) {
		v = strings.TrimSpace(v)
		if v != "" {
			result[strings.ToLower(v)] = struct{}{}
		}
	}
	return result
}
