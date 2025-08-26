// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"io"
	"os"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/apply"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/delete"
	getinfo "github.com/telekom/controlplane/rover-ctl/pkg/commands/get-info"
	resetsecret "github.com/telekom/controlplane/rover-ctl/pkg/commands/reset-secret"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/resource"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
)

func ErrorHandler(err error) {
	logger := log.L().WithName("error-handler")
	if err != nil {
		logger.Error(err, "An error occurred")
		os.Exit(1)
	} else {
		logger.Info("Command executed successfully")
	}
}

// NewRootCommand creates the root command for rover-ctl
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "roverctl",
		Short: "Rover Control Plane CLI",
		Long: `Rover Control Plane CLI tool for managing Control Plane resources.
This tool allows you to apply, delete, and manage resources in the Rover Control Plane.`,
		Version:       viper.GetString("version.full"),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Add global flags
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug mode")
	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))

	rootCmd.PersistentFlags().String("log-level", viper.GetString("log.level"), "Log level (debug, info, warn, error)")
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))

	rootCmd.PersistentFlags().String("log-format", viper.GetString("log.format"), "Log format (json or console)")
	viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))

	// Add output format flag
	rootCmd.PersistentFlags().String("format", viper.GetString("output.format"), "Output format (yaml|json)")
	viper.BindPFlag("output.format", rootCmd.PersistentFlags().Lookup("format"))

	// Add output file flag
	rootCmd.PersistentFlags().StringP("output", "o", "stdout", "Output file path")

	viper.BindPFlag("output.file", rootCmd.PersistentFlags().Lookup("output"))

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if viper.GetBool("debug") {
			viper.Set("log.level", "debug")
		}

		logger := log.NewLogger().WithName("rover-ctl")
		log.SetGlobalLogger(logger)
		ctx := logr.NewContext(cmd.Context(), logger)
		cmd.SetContext(ctx)

		outputFile := viper.GetString("output.file")
		if outputFile != "stdout" {
			file, err := os.OpenFile(outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return errors.Wrapf(err, "failed to open output file %s", outputFile)
			}
			cmd.SetOut(file)
		}

		handlers.RegisterHandlers()

		return nil
	}

	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		logger := logr.FromContextOrDiscard(cmd.Context())
		w := cmd.OutOrStdout()
		if w, ok := w.(io.Closer); ok {
			if err := w.Close(); err != nil {
				logger.Error(err, "Failed to close output file")
			}
		}
	}

	// Add subcommands
	rootCmd.AddCommand(
		apply.NewCommand(),
		delete.NewCommand(),
		resource.NewCommand(),
		getinfo.NewCommand(),
		resetsecret.NewCommand(),
	)

	return rootCmd
}
