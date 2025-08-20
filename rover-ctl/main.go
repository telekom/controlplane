// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/telekom/controlplane/rover-ctl/cmd"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
)

var (
	buildTime string
	gitCommit string
	version   string
)

func main() {
	config.Initialize()
	versionInfo := cmd.VersionInfo{
		BuildTime: buildTime,
		GitCommit: gitCommit,
		Version:   version,
	}
	rootCmd := cmd.NewRootCommand(versionInfo)
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)

	if err := rootCmd.Execute(); err != nil {
		cmd.ErrorHandler(err)
	}
}
