// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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

func main() {
	rootCmd := &cobra.Command{
		Use:   "e2e-tester",
		Short: "End-to-End Testing Suite for rover-ctl",
		Long: `A testing suite for rover-ctl commands that executes
commands, captures outputs, and compares them with expected snapshots.`,
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize global logger
			if err := logger.Initialize(logLevel, devMode); err != nil {
				fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
				os.Exit(1)
			}
			defer logger.Sync()
			// Setup signal handling
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

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

			var cfg config.Config
			if err := viper.Unmarshal(&cfg); err != nil {
				zap.L().Fatal("Error parsing config", zap.Error(err))
			}

			// Set verbose mode from flag
			cfg.Verbose = verboseMode
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

			result, err := r.Run(ctx)
			if err != nil {
				zap.L().Fatal("Test execution failed", zap.Error(err))
			}

			// Exit with code 1 if there were failures
			if result.TotalFailed > 0 || result.TotalErrors > 0 {
				os.Exit(1)
			}
		},
	}

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

	// These flags can be used together to run a specific suite in a specific environment
	// Additionally, suites can specify an environment in the config file using the 'environment' field

	// Execute
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
