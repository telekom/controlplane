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
	"go.uber.org/zap"
)

var (
	servePort int
	serveCmd  = &cobra.Command{
		Use:   "serve",
		Short: "Start the API server",
		Long:  `Start the HTTP API server for snapshot operations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			snapshotStore := NewStore(cfg.StorePath, noStore)
			if cleanStore {
				if err := snapshotStore.Clean(); err != nil {
					return fmt.Errorf("failed to clean snapshot store: %w", err)
				}
			}
			instances := orchestrator.NewFromConfig(cfg, snapshotStore)

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

			apiInstance := api.NewAPI(snapshotStore, instances)
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

	serveCmd.Flags().IntVar(&servePort, "port", 8080, "Port to listen on")
	serveCmd.Flags().BoolVar(&noStore, "no-store", false, "Do not store snapshots")
}
