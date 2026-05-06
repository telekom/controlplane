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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	cserver "github.com/telekom/controlplane/common-server/pkg/server"
	filesapi "github.com/telekom/controlplane/file-manager/api"
	"github.com/telekom/controlplane/rover-server/internal/file"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/telekom/controlplane/rover-server/internal/config"
	"github.com/telekom/controlplane/rover-server/internal/controller"
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

	s := server.Server{
		Config:              cfg,
		Log:                 log.Log,
		ApiSpecifications:   controller.NewApiSpecificationController(stores, cfg.OasLinting.ErrorMessage, cfg.OasLinting.Timeout, cfg.OasLinting.Async),
		Rovers:              controller.NewRoverController(stores),
		Roadmaps:            controller.NewRoadmapController(stores),
		EventSpecifications: controller.NewEventSpecificationController(stores),
		ApiChangelogs:       controller.NewApiChangelogController(stores),
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
