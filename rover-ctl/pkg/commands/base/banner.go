// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/telekom/controlplane/rover-ctl/pkg/config"
)

func PrintBanner(cmd *cobra.Command) {
	w := cmd.ErrOrStderr()

	// Content lines for application info
	titleLine := fmt.Sprintf("  %s  ", viper.GetString("name"))
	versionLine := fmt.Sprintf("  Version: %s  ", viper.GetString("version.semver"))
	gitCommitLine := fmt.Sprintf("  Git Commit: %s  ", viper.GetString("git.commit"))
	buildTimeLine := fmt.Sprintf("  Build Time: %s  ", viper.GetString("build.time"))

	// Prepare basic lines array
	lines := []string{titleLine, versionLine, gitCommitLine, buildTimeLine}

	// Try to get token, but don't print anything if error occurs
	token, err := config.GetToken()
	if err == nil && token != nil {
		// Only add user info if token is successfully retrieved
		userInfoLine := fmt.Sprintf("  User: %s--%s @ %s  ",
			token.Group, token.Team, token.Environment)
		generationInfoLine := fmt.Sprintf("  Token: %d (generated %s)  ",
			token.GeneratedAt, token.TimeSinceGenerated())

		lines = append(lines, userInfoLine, generationInfoLine)
	}

	// Find the longest line for border width
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

	// Add application info
	for _, line := range lines[:4] {
		banner += fmt.Sprintf("│%s│\n", line)
	}

	// Add separator and user info if token info is present
	if len(lines) > 4 {
		// Add separator
		banner += fmt.Sprintf("├%s┤\n", border)

		// User info lines
		for _, line := range lines[4:] {
			banner += fmt.Sprintf("│%s│\n", line)
		}
	}

	banner += fmt.Sprintf("└%s┘\n", border)

	_, _ = fmt.Fprintln(w, banner)
	_, _ = fmt.Fprintln(w)
}
