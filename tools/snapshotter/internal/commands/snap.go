// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/orchestrator"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	snapSourceKey  string
	snapRouteId    string
	snapConsumerId string
	snapStorePath  string
	snapRaw        bool
	snapLimit      int
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

			if err := validateSnapFlags(snapRouteId, snapRaw); err != nil {
				return err
			}

			if err := validateSnapLimit(snapLimit, cmd.Flags().Changed("limit"), snapRouteId, snapConsumerId); err != nil {
				return err
			}

			storePath, err := resolveStorePath(cfg, snapStorePath, noStore)
			if err != nil {
				return err
			}
			if !noStore {
				zap.L().Info("using snapshot store", zap.String("path", storePath))
			}
			snapshotStore := NewStore(storePath, noStore)
			if cleanStore {
				if err := snapshotStore.Clean(); err != nil {
					return fmt.Errorf("failed to clean snapshot store: %w", err)
				}
			}

			instances := orchestrator.NewFromConfig(cfg, snapshotStore)

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

			instance.Raw = snapRaw

			resourceType := "route"
			resId := snapRouteId
			if snapConsumerId != "" {
				resourceType = "consumer"
				resId = snapConsumerId
			}

			isGlobalSnapshotting := resId == ""

			instance.ReportResult = func(result diffmatcher.Result, snapID string) {
				zap.L().Info("snapshot result", zap.String("snapshotID", snapID), zap.Bool("changed", result.Changed))
				if !isGlobalSnapshotting {
					_ = formatOutput(result)
				}
			}

			if isGlobalSnapshotting {
				_, err = instance.Run(rootCtx, orchestrator.RunOptions{
					ResourceType: resourceType,
					Limit:        snapLimit,
				})
				return err
			} else {
				_, err = instance.Do(rootCtx, resourceType, resId)
				return err

			}
		},
	}
)

func init() {
	rootCmd.AddCommand(snapCmd)

	// Add local flags
	snapCmd.Flags().StringVar(&snapSourceKey, "source", "", "Source to snapshot from (only required if multiple sources are configured)")
	snapCmd.Flags().StringVar(&snapRouteId, "route", "", "ID of the route to snapshot")
	snapCmd.Flags().StringVar(&snapConsumerId, "consumer", "", "ID of the consumer to snapshot")
	snapCmd.Flags().BoolVar(&noStore, "no-store", false, "Do not store snapshots")
	snapCmd.Flags().BoolVar(&snapRaw, "raw", false, "Store the unmodified gateway state (no obfuscators/decoders); required for apply. May contain sensitive data.")
	snapCmd.Flags().IntVar(&snapLimit, "limit", 100, "Maximum number of resources for a global snapshot (without --route/--consumer); 0 = no limit")
	snapCmd.Flags().StringVar(&snapStorePath, "store", "", "Directory to store snapshots in (overrides storePath from the config file)")
}

// validateSnapFlags checks flag combinations of the snap command.
func validateSnapFlags(routeId string, raw bool) error {
	if raw && routeId == "" {
		return fmt.Errorf("--raw is only supported together with --route")
	}
	return nil
}

// validateSnapLimit checks the --limit flag; limitSet reports whether it was set explicitly.
func validateSnapLimit(limit int, limitSet bool, routeId, consumerId string) error {
	if limit < 0 {
		return fmt.Errorf("--limit must be >= 0")
	}
	if limitSet && (routeId != "" || consumerId != "") {
		return fmt.Errorf("--limit is only valid without --route/--consumer")
	}
	return nil
}
