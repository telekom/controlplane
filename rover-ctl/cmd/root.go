// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/apply"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/delete"
	getinfo "github.com/telekom/controlplane/rover-ctl/pkg/commands/get-info"
	getsecret "github.com/telekom/controlplane/rover-ctl/pkg/commands/get-secret"
	resetsecret "github.com/telekom/controlplane/rover-ctl/pkg/commands/reset-secret"
	"github.com/telekom/controlplane/rover-ctl/pkg/commands/resource"
	rotatesecret "github.com/telekom/controlplane/rover-ctl/pkg/commands/rotate-secret"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/pkg/log"
)

func ErrorHandler(err error, verbose bool) {
	logger := log.L().WithName("error-handler")
	if err != nil {
		logError(logger, err, verbose)
		os.Exit(1)
	} else {
		logger.Info("Command executed successfully")
	}
}

// logError logs the full error chain in verbose (--debug) mode and only the
// customer-facing root cause otherwise.
func logError(logger logr.Logger, err error, verbose bool) {
	if verbose {
		logger.Error(err, "An error occurred")
		return
	}
	logger.Error(nil, "An error occurred", "error", errors.Cause(err).Error())
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

	rootCmd.PersistentFlags().Duration("poll-delay", viper.GetDuration("poll.delay"), "Wait before polling the status after applying a resource (for example 3s, 0 to disable)")
	viper.BindPFlag("poll.delay", rootCmd.PersistentFlags().Lookup("poll-delay"))

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
		rotatesecret.NewCommand(),
		getsecret.NewCommand(),
	)

	return rootCmd
}

const (
	legacyNameFlag = "-name"
	modernNameFlag = "--name"
)

// NormalizeLegacyArgs rewrites the legacy single-dash long flag "-name" (and "-name=<value>")
// to "--name" for the resource command and its subcommands. All other arguments, values of
// flags and everything after the "--" terminator are passed through unchanged.
func NormalizeLegacyArgs(root *cobra.Command, args []string) []string {
	target := findTarget(root, args)
	if target == nil || !isResourceCommand(target) {
		return args
	}

	normalized := make([]string, len(args))
	copy(normalized, args)

	for i := 0; i < len(normalized); i++ {
		arg := normalized[i]
		switch {
		case arg == "--":
			return normalized
		case arg == legacyNameFlag:
			normalized[i] = modernNameFlag
			i++ // skip the flag value
		case strings.HasPrefix(arg, legacyNameFlag+"="):
			normalized[i] = "-" + arg
		case consumesNextArg(target, arg):
			i++
		}
	}

	return normalized
}

// findTarget resolves the command that will be executed for args.
func findTarget(root *cobra.Command, args []string) *cobra.Command {
	probe := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == legacyNameFlag || strings.HasPrefix(arg, legacyNameFlag+"=") {
			arg = "-" + arg
		}
		probe = append(probe, arg)
	}
	target, _, err := root.Find(probe)
	if err != nil {
		return nil
	}
	return target
}

func isResourceCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "resource" && c.HasParent() && !c.Parent().HasParent() {
			return true
		}
	}
	return false
}

// consumesNextArg reports whether arg is a flag of cmd whose value is the following argument.
func consumesNextArg(cmd *cobra.Command, arg string) bool {
	if !strings.HasPrefix(arg, "-") || arg == "-" || strings.Contains(arg, "=") {
		return false
	}
	if strings.HasPrefix(arg, "--") {
		return takesValue(lookupFlag(cmd, arg[2:], ""))
	}
	// Shorthand flags may be combined, e.g. "-dn <value>"; only the last one can take the next argument.
	shorthands := arg[1:]
	for i, r := range shorthands {
		if r > 127 {
			return false
		}
		if !takesValue(lookupFlag(cmd, "", string(r))) {
			continue
		}
		return i == len(shorthands)-1
	}
	return false
}

func lookupFlag(cmd *cobra.Command, name, shorthand string) *pflag.Flag {
	for _, fs := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		if name != "" {
			if f := fs.Lookup(name); f != nil {
				return f
			}
			continue
		}
		if f := fs.ShorthandLookup(shorthand); f != nil {
			return f
		}
	}
	return nil
}

func takesValue(f *pflag.Flag) bool {
	return f != nil && f.NoOptDefVal == ""
}
