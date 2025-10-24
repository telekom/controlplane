// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	configPath string
	rootCmd    = &cobra.Command{
		Use:   "snapshotter",
		Short: "Snapshotter tool for the control plane",
		Long: `A CLI tool for taking, comparing and serving snapshots of the control plane.
It supports taking snapshots, comparing snapshots, and serving an API for snapshot operations.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Setup logging for all commands
			zap.ReplaceGlobals(zap.Must(zap.NewDevelopment()))
			return nil
		},
	}
)

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to the configuration file")
}
