// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidConfig(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{
			Binary: "/usr/local/bin/snapshotter",
		},
		RoverCtl: RoverCtlConfig{
			Binary: "roverctl",
		},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name:         "test-suite",
				Environments: []string{"test-env"},
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestMissingRoverCtlBinary(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing RoverCtl.Binary")
	}
	if !strings.Contains(err.Error(), "roverctl") && !strings.Contains(err.Error(), "binary") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMissingSnapshotter(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{}, // Neither URL nor Binary
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing snapshotter config")
	}
	if !strings.Contains(err.Error(), "Snapshotter") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSnapshotterWithURL(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{URL: "http://localhost:8080"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config with snapshotter URL, got error: %v", err)
	}
}

func TestInvalidCaseType(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version", Type: "invalid-type"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid case type")
	}
	if !strings.Contains(err.Error(), "must be one of: roverctl snapshot") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidCaseTypes(t *testing.T) {
	types := []string{"roverctl", "snapshot", ""}

	for _, typ := range types {
		cfg := &Config{
			Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
			RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
			Environments: []Environments{
				{Name: "test-env", Token: "test-token"},
			},
			Suites: []Suite{
				{
					Name: "test-suite",
					Cases: []*Case{
						{Name: "test-case", Command: "--version", Type: typ},
					},
				},
			},
		}

		if err := cfg.Validate(); err != nil {
			t.Errorf("expected valid case type '%s', got error: %v", typ, err)
		}
	}
}

func TestInvalidRunPolicy(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version", RunPolicy: "invalid"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid run policy")
	}
	if !strings.Contains(err.Error(), "run_policy") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidRunPolicies(t *testing.T) {
	policies := []RunPolicy{RunPolicyNormal, RunPolicyCritical, RunPolicyAlways, ""}

	for _, policy := range policies {
		cfg := &Config{
			Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
			RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
			Environments: []Environments{
				{Name: "test-env", Token: "test-token"},
			},
			Suites: []Suite{
				{
					Name: "test-suite",
					Cases: []*Case{
						{Name: "test-case", Command: "--version", RunPolicy: policy},
					},
				},
			},
		}

		if err := cfg.Validate(); err != nil {
			t.Errorf("expected valid run_policy '%s', got error: %v", policy, err)
		}
	}
}

func TestEnvironmentCrossReference(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "env-a", Token: "token-a"},
		},
		Suites: []Suite{
			{
				Name:         "test-suite",
				Environments: []string{"nonexistent-env"},
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown environment reference")
	}
	if !strings.Contains(err.Error(), "nonexistent-env") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCaseEnvironmentCrossReference(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "env-a", Token: "token-a"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version", Environment: "unknown-env"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown case environment reference")
	}
	if !strings.Contains(err.Error(), "unknown-env") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDuplicateEnvironmentNames(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "duplicate", Token: "token-1"},
			{Name: "duplicate", Token: "token-2"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate environment names")
	}
	if !strings.Contains(err.Error(), "unique") && !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDuplicateSuiteNames(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "duplicate",
				Cases: []*Case{
					{Name: "test-case-1", Command: "--version"},
				},
			},
			{
				Name: "duplicate",
				Cases: []*Case{
					{Name: "test-case-2", Command: "--help"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate suite names")
	}
	if !strings.Contains(err.Error(), "unique") && !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMissingCaseName(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Command: "--version"}, // Missing Name
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing case name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMissingCommand(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case"}, // Missing Command
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEmptySuites(t *testing.T) {
	cfg := &Config{
		Snapshotter:  SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:     RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{{Name: "test-env", Token: "test-token"}},
		Suites:       []Suite{},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty suites")
	}
	if !strings.Contains(err.Error(), "at least 1") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEmptyCases(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name:  "test-suite",
				Cases: []*Case{},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty cases")
	}
	if !strings.Contains(err.Error(), "at least 1") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNegativeWaitDuration(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{
						Name:       "test-case",
						Command:    "--version",
						WaitBefore: -1 * time.Second,
					},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative wait duration")
	}
	if !strings.Contains(err.Error(), "wait_before") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAggregatedErrors(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{}, // Missing URL and Binary
		RoverCtl:    RoverCtlConfig{},    // Missing Binary
		Environments: []Environments{
			{Name: "test-env"}, // Missing Token
		},
		Suites: []Suite{
			{
				Name:         "",                              // Missing Name
				Environments: []string{"nonexistent"},         // Invalid reference
				Cases:        []*Case{{Type: "invalid-type"}}, // Invalid type, missing name & command
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected multiple validation errors")
	}

	errStr := err.Error()
	// Check that multiple errors are reported
	if !strings.Contains(errStr, "configuration validation failed:") {
		t.Errorf("expected aggregated error format, got: %v", err)
	}
	// Count bullet points (error items)
	bulletCount := strings.Count(errStr, "\n  - ")
	if bulletCount < 3 {
		t.Errorf("expected at least 3 errors, got %d in: %v", bulletCount, err)
	}
}

func TestMissingSuiteName(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "", // Missing Name
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing suite name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMissingEnvironmentName(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "", Token: "test-token"}, // Missing Name
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing environment name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMissingEnvironmentToken(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: ""}, // Missing Token
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing environment token")
	}
	if !strings.Contains(err.Error(), "token is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEmptyEnvironments(t *testing.T) {
	cfg := &Config{
		Snapshotter:  SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:     RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty environments")
	}
	if !strings.Contains(err.Error(), "at least 1") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestVariableValidation(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{
				Name:  "test-env",
				Token: "test-token",
				Variables: []Variable{
					{Name: "", Value: "value"}, // Missing variable name
				},
			},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing variable name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidConfigWithAllFields(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{
			URL:    "http://localhost:8080",
			Binary: "snapshotter",
		},
		RoverCtl: RoverCtlConfig{
			Binary:      "roverctl",
			DownloadURL: "https://example.com/roverctl",
		},
		Environments: []Environments{
			{
				Name:  "env-a",
				Token: "token-a",
				Variables: []Variable{
					{Name: "VAR1", Value: "value1"},
					{Name: "VAR2", Value: "value2"},
				},
			},
			{
				Name:  "env-b",
				Token: "token-b",
			},
		},
		Suites: []Suite{
			{
				Name:         "suite-1",
				Description:  "Test suite 1",
				Environments: []string{"env-a", "env-b"},
				Cases: []*Case{
					{
						Name:        "case-1",
						Description: "Test case 1",
						Type:        "roverctl",
						RunPolicy:   RunPolicyCritical,
						Command:     "--version",
						Compare:     true,
						WaitBefore:  5 * time.Second,
						WaitAfter:   2 * time.Second,
						Selector:    "$.version",
					},
					{
						Name:        "case-2",
						Type:        "snapshot",
						RunPolicy:   RunPolicyAlways,
						Command:     "snap --source test",
						Environment: "env-a",
					},
				},
			},
		},
		Verbose: true,
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config with all fields, got error: %v", err)
	}
}

func TestValidateNilConfig(t *testing.T) {
	err := ValidateConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestInvalidSnapshotterURL(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{URL: "not-a-valid-url"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid snapshotter URL")
	}
	if !strings.Contains(err.Error(), "url") || !strings.Contains(err.Error(), "URL") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestInvalidRoverCtlDownloadURL(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl", DownloadURL: "invalid-url"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name: "test-suite",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid RoverCtl DownloadURL")
	}
	if !strings.Contains(err.Error(), "url") || !strings.Contains(err.Error(), "URL") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDeepCopyWithNilCases(t *testing.T) {
	suite := &Suite{
		Name:         "test-suite",
		Description:  "Test suite with nil cases",
		Cases:        []*Case{nil, {Name: "valid-case", Command: "--version"}, nil},
		Environments: []string{"env-a"},
	}

	// Should not panic
	copied := suite.DeepCopy()

	if copied.Name != suite.Name {
		t.Errorf("expected name %s, got %s", suite.Name, copied.Name)
	}
	if len(copied.Cases) != len(suite.Cases) {
		t.Errorf("expected %d cases, got %d", len(suite.Cases), len(copied.Cases))
	}
	if copied.Cases[0] != nil {
		t.Error("expected first case to be nil")
	}
	if copied.Cases[1] == nil || copied.Cases[1].Name != "valid-case" {
		t.Error("expected second case to be valid")
	}
	if copied.Cases[2] != nil {
		t.Error("expected third case to be nil")
	}
}

// Tests for external suite files feature

func TestSuiteIsExternal(t *testing.T) {
	tests := []struct {
		name     string
		suite    Suite
		expected bool
	}{
		{
			name:     "Suite with filepath is external",
			suite:    Suite{Name: "external-suite", Filepath: "./suites/test.yaml"},
			expected: true,
		},
		{
			name:     "Suite without filepath is not external",
			suite:    Suite{Name: "inline-suite", Cases: []*Case{{Name: "case-1", Command: "--version"}}},
			expected: false,
		},
		{
			name:     "Suite with empty filepath is not external",
			suite:    Suite{Name: "inline-suite", Filepath: "", Cases: []*Case{{Name: "case-1", Command: "--version"}}},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.suite.IsExternal() != tc.expected {
				t.Errorf("IsExternal() = %v, want %v", tc.suite.IsExternal(), tc.expected)
			}
		})
	}
}

func TestSuiteFilepathAndCasesExclusive(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name:     "invalid-suite",
				Filepath: "./suites/test.yaml",
				Cases: []*Case{
					{Name: "test-case", Command: "--version"},
				},
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for suite with both filepath and cases")
	}
	if !strings.Contains(err.Error(), "filepath") || !strings.Contains(err.Error(), "cases") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSuiteFilepathOnly(t *testing.T) {
	// Suite with only filepath should be valid (cases not required when filepath is set)
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name:     "external-suite",
				Filepath: "./suites/test.yaml",
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected valid config with filepath only, got error: %v", err)
	}
}

func TestSuiteLoadExternalFile(t *testing.T) {
	suite := Suite{
		Name:     "my-external-suite",
		Filepath: "testdata/valid-external-suite.yaml",
	}

	// Load from the testdata directory
	err := suite.Load(".")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify the suite was loaded correctly
	if suite.Name != "my-external-suite" {
		t.Errorf("Name should be preserved from root config, got %s", suite.Name)
	}
	if suite.Filepath != "" {
		t.Errorf("Filepath should be cleared after load, got %s", suite.Filepath)
	}
	if suite.Description != "External test suite" {
		t.Errorf("Description = %q, want %q", suite.Description, "External test suite")
	}
	if len(suite.Cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(suite.Cases))
	}
	if suite.Cases[0].Name != "external-case-1" {
		t.Errorf("Cases[0].Name = %s, want external-case-1", suite.Cases[0].Name)
	}
	if len(suite.Environments) != 1 || suite.Environments[0] != "test-env" {
		t.Errorf("Environments = %v, want [test-env]", suite.Environments)
	}
}

func TestSuiteLoadNoFilepath(t *testing.T) {
	suite := Suite{
		Name:  "inline-suite",
		Cases: []*Case{{Name: "case-1", Command: "--version"}},
	}

	// Load should be a no-op when no filepath is set
	err := suite.Load(".")
	if err != nil {
		t.Fatalf("Load() should succeed for suite without filepath: %v", err)
	}

	// Suite should remain unchanged
	if len(suite.Cases) != 1 {
		t.Errorf("expected 1 case, got %d", len(suite.Cases))
	}
}

func TestSuiteLoadNonExistentFile(t *testing.T) {
	suite := Suite{
		Name:     "missing-suite",
		Filepath: "testdata/non-existent.yaml",
	}

	err := suite.Load(".")
	if err == nil {
		t.Fatal("Load() should fail for non-existent file")
	}
	if !strings.Contains(err.Error(), "failed to read suite file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSuiteLoadAbsolutePath(t *testing.T) {
	// Get absolute path to testdata
	suite := Suite{
		Name:     "absolute-path-suite",
		Filepath: "testdata/valid-external-suite.yaml",
	}

	// When configDir is empty, relative path should still work from current dir
	err := suite.Load(".")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if len(suite.Cases) != 2 {
		t.Errorf("expected 2 cases, got %d", len(suite.Cases))
	}
}

func TestConfigLoadSuites(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name:     "external-suite",
				Filepath: "testdata/valid-external-suite.yaml",
			},
			{
				Name: "inline-suite",
				Cases: []*Case{
					{Name: "inline-case", Command: "--help"},
				},
			},
		},
	}

	err := cfg.LoadSuites(".")
	if err != nil {
		t.Fatalf("LoadSuites() failed: %v", err)
	}

	// Verify external suite was loaded
	if len(cfg.Suites[0].Cases) != 2 {
		t.Errorf("external suite should have 2 cases, got %d", len(cfg.Suites[0].Cases))
	}
	if cfg.Suites[0].Filepath != "" {
		t.Errorf("Filepath should be cleared after load")
	}

	// Verify inline suite remains unchanged
	if len(cfg.Suites[1].Cases) != 1 {
		t.Errorf("inline suite should have 1 case, got %d", len(cfg.Suites[1].Cases))
	}
}

func TestConfigLoadSuitesWithError(t *testing.T) {
	cfg := &Config{
		Snapshotter: SnapshotterConfig{Binary: "snapshotter"},
		RoverCtl:    RoverCtlConfig{Binary: "roverctl"},
		Environments: []Environments{
			{Name: "test-env", Token: "test-token"},
		},
		Suites: []Suite{
			{
				Name:     "missing-suite",
				Filepath: "testdata/non-existent.yaml",
			},
		},
	}

	err := cfg.LoadSuites(".")
	if err == nil {
		t.Fatal("LoadSuites() should fail for non-existent file")
	}
}
