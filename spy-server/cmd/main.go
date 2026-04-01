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
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/telekom/controlplane/spy-server/internal/config"
	"github.com/telekom/controlplane/spy-server/internal/controller"
	"github.com/telekom/controlplane/spy-server/internal/server"
	"github.com/telekom/controlplane/spy-server/pkg/log"
	"github.com/telekom/controlplane/spy-server/pkg/store"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(errors.Wrap(err, "failed to load configuration"))
	}

	log.Init()
	rootCtx := logr.NewContext(context.Background(), log.Log)

	stores := store.NewStores(rootCtx, kconfig.GetConfigOrDie(), cfg)

	appCfg := cserver.NewAppConfig()
	appCfg.CtxLog = log.Log
	app := cserver.NewAppWithConfig(appCfg)

	probesCtrl := cserver.NewProbesController()
	probesCtrl.Register(app, cserver.ControllerOpts{})

	s := server.Server{
		Config:             cfg,
		Log:                log.Log,
		ApiExposures:       controller.NewApiExposureController(stores),
		ApiSubscriptions:   controller.NewApiSubscriptionController(stores),
		Applications:       controller.NewApplicationController(stores),
		EventExposures:     controller.NewEventExposureController(stores),
		EventSubscriptions: controller.NewEventSubscriptionController(stores),
		EventTypes:         controller.NewEventTypeController(stores),
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
