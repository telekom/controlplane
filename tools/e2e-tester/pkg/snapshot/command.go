// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/telekom/controlplane/tools/e2e-tester/pkg/obfuscator"
)

// CommandOutput represents the output of a command execution
type CommandOutput struct {
	Command     string `yaml:"command" json:"command"`
	ExitCode    int    `yaml:"exit_code" json:"exit_code"`
	Stdout      string `yaml:"stdout" json:"stdout"`
	Stderr      string `yaml:"stderr" json:"stderr"`
	Environment string `yaml:"environment" json:"environment"`
	Duration    string `yaml:"duration" json:"duration"`
}

// CommandSnapshot represents a snapshot of a command execution
type CommandSnapshot struct {
	Id      string        `yaml:"id" json:"id"`
	Version int           `yaml:"version" json:"version"`
	Output  CommandOutput `yaml:"output" json:"output"`
}

// ID returns the snapshot ID
func (s *CommandSnapshot) ID() string {
	return s.Id
}

// String returns a string representation of the snapshot for comparison
func (s *CommandSnapshot) String() string {
	// Marshal the output with literal block style for multiline strings
	relevantOutput := map[string]any{
		"command":     s.Output.Command,
		"exit_code":   s.Output.ExitCode,
		"stdout":      s.Output.Stdout,
		"stderr":      s.Output.Stderr,
		"environment": s.Output.Environment,
	}
	data, err := yaml.MarshalWithOptions(relevantOutput, yaml.UseLiteralStyleIfMultiline(true))
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal command output: %v", err))
	}

	// Apply obfuscation patterns to the snapshot data
	obfuscatedData, err := obfuscateSnapshotData(data)
	if err != nil {
		panic(fmt.Sprintf("Failed to obfuscate snapshot data: %v", err))
	}

	return string(obfuscatedData)
}

// MakeSnapshotID creates a unique ID for a command snapshot
// using suite name, environment name, case index and case name
func MakeSnapshotID(suite, env, caseIndex, caseName string) string {
	// Sanitize strings for use in filenames
	sanitizePath := func(s string) string {
		s = strings.ReplaceAll(s, " ", "_")
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, "\\", "_")
		return s
	}

	sanitizedSuite := sanitizePath(suite)
	sanitizedCaseName := sanitizePath(caseName)

	return fmt.Sprintf("%s_%s_%s_%s", sanitizedSuite, env, caseIndex, sanitizedCaseName)
}

// obfuscateSnapshotData applies obfuscation patterns to snapshot data
// before comparison to eliminate false differences from dynamic data
func obfuscateSnapshotData(data []byte) ([]byte, error) {
	return obfuscator.ObfuscateSnapshot(data)
}
