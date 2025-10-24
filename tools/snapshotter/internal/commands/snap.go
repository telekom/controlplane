// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	snapSourceKey  string
	snapRouteId    string
	snapConsumerId string
	snapStorePath  string
	snapCmd        = &cobra.Command{
		Use:   "snap",
		Short: "Take snapshots of resources",
		Long:  `Take snapshots of routes or consumers from the configured sources.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rootCtx := signals.SetupSignalHandler()

			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if snapRouteId != "" && snapConsumerId != "" {
				return fmt.Errorf("only one of route or consumer ID can be provided")
			}

			store := store.NewFileStore(snapStorePath)

			instances := orchestrator.NewFromConfig(cfg, store)

			var instance *orchestrator.Orchestrator
			var exists bool
			if snapSourceKey != "" {
				instance, exists = instances[snapSourceKey]
			} else if len(instances) == 1 {
				for _, inst := range instances {
					instance = inst
				}
				exists = true
			} else {
				return fmt.Errorf("multiple sources configured, but no source key provided")
			}

			if !exists {
				return fmt.Errorf("no orchestrator found for source key: %s", snapSourceKey)
			}

			resourceType := "route"
			resId := snapRouteId
			if snapConsumerId != "" {
				resourceType = "consumer"
				resId = snapConsumerId
			}

			instance.ReportResult = func(result diffmatcher.Result, snapID string) {
				zap.L().Info("snapshot result", zap.String("snapshotID", snapID), zap.Bool("changed", result.Changed))
				if result.Changed {
					_, _ = fmt.Fprint(os.Stdout, result.Text)
				}
			}

			_, err = instance.Do(rootCtx, resourceType, resId)
			return err
		},
	}
)

func init() {
	rootCmd.AddCommand(snapCmd)

	// Add local flags
	snapCmd.Flags().StringVar(&snapSourceKey, "source", "", "Source to snapshot from (only required if multiple sources are configured)")
	snapCmd.Flags().StringVar(&snapRouteId, "route", "", "ID of the route to snapshot")
	snapCmd.Flags().StringVar(&snapConsumerId, "consumer", "", "ID of the consumer to snapshot")
	snapCmd.Flags().StringVar(&snapStorePath, "store", "./snapshots", "Path to the snapshot store")
}
