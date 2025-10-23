// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/source"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.LoadConfig("")
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(zap.Must(zap.NewDevelopment()))

	store := store.NewFileStore("./snapshots")

	instances := map[string]*orchestrator.Orchestrator{}

	for key, sourceCfg := range cfg.Sources {
		zap.L().Info("setting up source", zap.String("key", key))
		kongSource, err := source.NewKongSourceFromConfig(sourceCfg)
		if err != nil {
			panic(err)
		}
		kongSource.SetTags(sourceCfg.Tags)
		instances[key] = orchestrator.NewOrchestrator(kongSource, store, sourceCfg.Obfuscators)
	}

	errHandler := func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		var e *fiber.Error
		if errors.As(err, &e) {
			code = e.Code
		}
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.Status(code).JSON(map[string]any{
			"error": err.Error(),
		})
	}
	app := fiber.New(fiber.Config{
		ErrorHandler: errHandler,
	})
	api := api.NewAPI(store, instances)
	api.RegisterRoutes(app)

	if err := app.Listen(":8080"); err != nil {
		zap.L().Fatal("failed to start server", zap.Error(err))
	}

}
