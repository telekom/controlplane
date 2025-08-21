// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/cmd"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
)

var (
	buildTime = "unknown"
	gitCommit = "unknown"
	version   = "0.0.0"
)

func main() {
	config.Initialize()
	viper.Set("name", "Rover Control Plane CLI")
	viper.Set("version.semver", version)
	viper.Set("build.time", buildTime)
	viper.Set("git.commit", gitCommit)
	viper.Set("version.full", fmt.Sprintf("%s (build-time: %s, git-commit: %s)", version, buildTime, gitCommit))

	rootCmd := cmd.NewRootCommand()
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)

	if err := rootCmd.Execute(); err != nil {
		cmd.ErrorHandler(err)
	}
}
