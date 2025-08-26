// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func PrintBanner(cmd *cobra.Command) {
	w := cmd.ErrOrStderr()

	// Content lines
	titleLine := fmt.Sprintf("  %s  ", viper.GetString("name"))
	versionLine := fmt.Sprintf("  Version: %s  ", viper.GetString("version.semver"))
	gitCommitLine := fmt.Sprintf("  Git Commit: %s  ", viper.GetString("git.commit"))
	buildTimeLine := fmt.Sprintf("  Build Time: %s  ", viper.GetString("build.time"))

	// Find the longest line for border width
	lines := []string{titleLine, versionLine, gitCommitLine, buildTimeLine}
	maxWidth := 0
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}

	minWidth := 40 // Minimum width for the banner
	if maxWidth < minWidth {
		maxWidth = minWidth
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
