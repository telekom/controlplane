// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	cmpAId, cmpBId string
	cmpMustExist   bool
	cmpCmd         = &cobra.Command{
		Use:   "cmp",
		Short: "Compare snapshots",
		Long:  `Compare two snapshots from the snapshot store.`,
		RunE: func(cmd *cobra.Command, args []string) error {

			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			rootCtx := signals.SetupSignalHandler()

			snapshotStore := NewStore(cfg.StorePath, noStore)
			if cleanStore {
				if err := snapshotStore.Clean(); err != nil {
					return fmt.Errorf("failed to clean snapshot store: %w", err)
				}
			}

			a := &snapshot.Snapshot{}
			err = snapshotStore.GetLatest(rootCtx, cmpAId, a)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					return fmt.Errorf("failed to get snapshot A: %w", err)
				}
				if cmpMustExist {
					return fmt.Errorf("snapshot A not found: %s", cmpAId)
				}
			}

			b := &snapshot.Snapshot{}
			err = snapshotStore.GetLatest(rootCtx, cmpBId, b)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					return fmt.Errorf("failed to get snapshot B: %w", err)
				}
				if cmpMustExist {
					return fmt.Errorf("snapshot B not found: %s", cmpBId)
				}
			}

			diff := diffmatcher.Compare(a, b)
			return formatOutput(diff)
		},
	}
)

func init() {
	rootCmd.AddCommand(cmpCmd)

	// Add local flags
	cmpCmd.Flags().StringVar(&cmpAId, "a", "", "ID of the first snapshot to compare")
	cmpCmd.Flags().StringVar(&cmpBId, "b", "", "ID of the second snapshot to compare")
	cmpCmd.Flags().BoolVar(&cmpMustExist, "must", false, "If set, both snapshots must exist")

	// Mark required flags
	cmpCmd.MarkFlagRequired("a")
	cmpCmd.MarkFlagRequired("b")
}
