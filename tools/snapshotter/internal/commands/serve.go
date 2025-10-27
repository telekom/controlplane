// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/cobra"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

var (
	serveStorePath string
	servePort      int
	serveCmd       = &cobra.Command{
		Use:   "serve",
		Short: "Start the API server",
		Long:  `Start the HTTP API server for snapshot operations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			store := store.NewFileStore[*snapshot.Snapshot](serveStorePath)
			instances := orchestrator.NewFromConfig(cfg, store)

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

			apiInstance := api.NewAPI(store, instances)
			apiInstance.RegisterRoutes(app)

			addr := fmt.Sprintf(":%d", servePort)
			zap.L().Info("starting server", zap.String("address", addr))
			if err := app.Listen(addr); err != nil {
				return fmt.Errorf("failed to start server: %w", err)
			}

			return nil
		},
	}
)

func init() {
	rootCmd.AddCommand(serveCmd)

	// Add local flags
	serveCmd.Flags().StringVar(&serveStorePath, "store", "./snapshots", "Path to the snapshot store")
	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Port to listen on")
}
