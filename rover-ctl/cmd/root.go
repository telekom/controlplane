// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

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

type VersionInfo struct {
	BuildTime string
	GitCommit string
	Version   string
}

func (i VersionInfo) String() string {
	return fmt.Sprintf("%s (build-time: %s, git-commit: %s)", i.Version, i.BuildTime, i.GitCommit)
}

// NewRootCommand creates the root command for rover-ctl
func NewRootCommand(versionInfo VersionInfo) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "roverctl",
		Short: "Rover Control Plane CLI",
		Long: `Rover Control Plane CLI tool for managing Control Plane resources.
This tool allows you to apply, delete, and manage resources in the Rover Control Plane.`,
		Version:       versionInfo.String(),
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

		printBanner(cmd, "Rover Control Plane CLI", versionInfo)

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

func printBanner(cmd *cobra.Command, title string, versionInfo VersionInfo) {
	w := cmd.ErrOrStderr()

	// Content lines
	titleLine := fmt.Sprintf("  %s  ", title)
	versionLine := fmt.Sprintf("  Version: %-20s  ", versionInfo.Version)
	gitCommitLine := fmt.Sprintf("  Git Commit: %-16s  ", versionInfo.GitCommit)
	buildTimeLine := fmt.Sprintf("  Build Time: %-17s  ", versionInfo.BuildTime)

	// Find the longest line for border width
	lines := []string{titleLine, versionLine, gitCommitLine, buildTimeLine}
	maxWidth := 0
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}

	// Create horizontal border
	border := strings.Repeat("=", maxWidth)

	// Pad lines to max width
	for i, line := range lines {
		padding := strings.Repeat(" ", maxWidth-len(line))
		lines[i] = line + padding
	}

	// Construct banner
	banner := fmt.Sprintf("\n┌%s┐\n", border)
	for _, line := range lines {
		banner += fmt.Sprintf("│%s│\n", line)
	}
	banner += fmt.Sprintf("└%s┘\n", border)

	fmt.Fprintln(w, banner)
	fmt.Fprintln(w, "Use 'roverctl --help' for more information.")
	fmt.Fprintln(w)
}
