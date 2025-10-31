// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"go.uber.org/zap"
)

type OutputFormat string

const (
	OutputFormatText OutputFormat = "text" // will display the diff
	OutputFormatYAML OutputFormat = "yaml" // will output the result in YAML format
	OutputFormatJSON OutputFormat = "json" // will output the result in JSON format
)

var (
	outputFormat    OutputFormat
	outputFormatStr string
	cleanStore      bool
	configPath      string
	rootCmd         = &cobra.Command{
		Use:   "snapshotter",
		Short: "Snapshotter tool for the control plane",
		Long: `A CLI tool for taking, comparing and serving snapshots of the control plane.
It supports taking snapshots, comparing snapshots, and serving an API for snapshot operations.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Setup logging for all commands
			zap.ReplaceGlobals(zap.Must(zap.NewDevelopment()))

			outputFormat = OutputFormat(outputFormatStr)

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
	rootCmd.PersistentFlags().StringVar(&outputFormatStr, "format", "text", "Output format: text, yaml, json")
	rootCmd.PersistentFlags().BoolVar(&cleanStore, "clean-store", false, "Clean the snapshot store before executing the command")
}

// formatOutput formats the diffmatcher.Result according to the specified output format
func formatOutput(result diffmatcher.Result) error {
	switch outputFormat {
	case OutputFormatText:
		if result.Changed {
			_, _ = fmt.Fprint(os.Stdout, result.Text)
		} else {
			zap.L().Info("snapshots are identical")
		}
		return nil
	case OutputFormatYAML:
		data, err := yaml.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal result to YAML: %w", err)
		}
		_, _ = fmt.Fprint(os.Stdout, string(data))
		return nil
	case OutputFormatJSON:
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result to JSON: %w", err)
		}
		_, _ = fmt.Fprint(os.Stdout, string(data))
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}
