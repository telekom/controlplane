// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/config"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/logger"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/report"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/runner"
	"go.uber.org/zap"
)

var (
	cfgFile      string
	updateMode   bool
	continueFlag bool
	snapshotsDir string
	logLevel     string
	devMode      bool
	verboseMode  bool
	suiteFilter  string
	envFilter    string
)

var (
	cfg config.Config

	runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the end-to-end tests",
		Long:  `Executes the end-to-end tests as per the provided configuration.`,
		Run: func(cmd *cobra.Command, args []string) {

			var reporter report.Reporter = report.NewConsoleReporter(os.Stderr, verboseMode)

			// Create and run the test runner
			r := runner.NewRunner(&cfg, runner.RunnerOptions{
				UpdateMode:     updateMode,
				ContinueOnFail: continueFlag,
				SnapshotsDir:   snapshotsDir,
				SuiteFilter:    suiteFilter,
				EnvFilter:      envFilter,
				Reporter:       reporter,
			})

			result, err := r.Run(cmd.Context())
			if err != nil {
				zap.L().Fatal("Test execution failed", zap.Error(err))
			}

			// Exit with code 1 if there were failures
			if result.TotalFailed > 0 || result.TotalErrors > 0 {
				os.Exit(1)
			}
		},
	}

	verifyCmd = &cobra.Command{
		Use:   "verify",
		Short: "Verify the end-to-end tests",
		Long:  `Verifies the configuration and setup for the end-to-end tests without executing them.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Count total test cases across all suites
			totalCases := 0
			for _, suite := range cfg.Suites {
				totalCases += len(suite.Cases)
			}

			zap.L().Info("Verification completed successfully. Configuration is valid.",
				zap.Int("suites", len(cfg.Suites)),
				zap.Int("cases", totalCases),
				zap.Int("environments", len(cfg.Environments)),
			)
		},
	}

	rootCmd = &cobra.Command{
		Use:   "e2e-tester",
		Short: "End-to-End Testing Suite for rover-ctl",
		Long: `A testing suite for rover-ctl commands that executes
commands, captures outputs, and compares them with expected snapshots.`,

		Run: func(cmd *cobra.Command, args []string) {
			runCmd.Run(cmd, args)
		},

		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			logger.Sync()
		},

		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Initialize global logger
			if err := logger.Initialize(logLevel, devMode); err != nil {
				fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
				os.Exit(1)
			}
			// Setup signal handling
			ctx, cancel := context.WithCancel(context.Background())

			cmd.SetContext(ctx)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			go func() {
				<-sigCh
				zap.L().Info("Received interrupt signal. Cleaning up...")
				cancel()
				time.Sleep(1 * time.Second)
				os.Exit(1)
			}()

			// Load configuration
			if err := config.LoadConfig(cfgFile); err != nil {
				zap.L().Fatal("Error loading config", zap.Error(err))
			}

			if err := viper.Unmarshal(&cfg); err != nil {
				zap.L().Fatal("Error parsing config", zap.Error(err))
			}

			// Validate configuration (initial)
			if err := cfg.Validate(); err != nil {
				cmd.ErrOrStderr().Write(fmt.Appendf(nil, "%v\n", err))
				os.Exit(1)
			}

			configDir := filepath.Dir(viper.ConfigFileUsed())
			if err := cfg.LoadExternalSuites(configDir); err != nil {
				zap.L().Fatal("Error loading test suites", zap.Error(err))
			}

			// Validate configuration (after loading suites)
			if err := cfg.Validate(); err != nil {
				cmd.ErrOrStderr().Write(fmt.Appendf(nil, "%v\n", err))
				os.Exit(1)
			}

			// Set verbose mode from flag
			cfg.Verbose = verboseMode
		},
	}
)

func main() {
	// Add flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is e2e-test-config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&updateMode, "update", false, "update snapshots instead of comparing")
	rootCmd.PersistentFlags().BoolVar(&continueFlag, "continue", false, "continue execution even if tests fail")
	rootCmd.PersistentFlags().StringVar(&snapshotsDir, "snapshots-dir", "snapshots", "directory to store snapshots")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&devMode, "dev", false, "enable development mode logging")
	rootCmd.PersistentFlags().BoolVar(&verboseMode, "verbose", false, "show detailed output including complete diffs")

	// Suite and environment filtering flags
	rootCmd.PersistentFlags().StringVar(&suiteFilter, "suite", "", "run only the specified test suite (by name)")
	rootCmd.PersistentFlags().StringVar(&envFilter, "env", "", "run tests only in the specified environment (by name)")

	// Add commands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(verifyCmd)

	// Execute
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
