// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEndToEnd(t *testing.T) {
	// Skip this test in short mode
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	// Create a temporary directory for test
	testDir, err := os.MkdirTemp("", "e2e-tester-e2e-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create a mock rover-ctl script
	mockRoverCtl := filepath.Join(testDir, "mock-roverctl")
	err = os.WriteFile(mockRoverCtl, []byte(`#!/bin/sh
echo "Command: $@"
echo "Environment: $ROVER_TOKEN"
exit 0
`), 0755)
	if err != nil {
		t.Fatalf("Failed to create mock rover-ctl: %v", err)
	}

	// Create a test config file
	cfgPath := filepath.Join(testDir, "test-config.yaml")
	err = os.WriteFile(cfgPath, []byte(`snapshotter:
  url: "http://localhost:8080"

roverctl:
  binary: "`+mockRoverCtl+`"

environments:
  - name: "test-env"
    token: "test-token"

suites:
  - name: "test-suite"
    environments:
      - "test-env"
    cases:
      - name: "version-check"
        must_pass: true
        command: "version"
        compare: true
      - name: "get-resources"
        must_pass: false
        command: "get resources"
        compare: true
`), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Create snapshot directory
	snapshotDir := filepath.Join(testDir, "snapshots")
	err = os.MkdirAll(snapshotDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create snapshot dir: %v", err)
	}

	// Run the test in update mode first
	ctx := context.Background()
	runTest(t, ctx, cfgPath, snapshotDir, true)

	// Now run in compare mode
	runTest(t, ctx, cfgPath, snapshotDir, false)
}

func runTest(t *testing.T, ctx context.Context, cfgPath, snapshotDir string, updateMode bool) {
	// Load config
	cmd := exec.Command("go", "run", "../main.go", "--config", cfgPath, "--snapshots-dir", snapshotDir)
	if updateMode {
		cmd.Args = append(cmd.Args, "--update")
	}

	output, err := cmd.CombinedOutput()
	t.Logf("Test output: %s", string(output))

	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	// Verify snapshots were created
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatalf("Failed to read snapshot dir: %v", err)
	}

	if len(entries) == 0 {
		t.Errorf("No snapshots created")
	}
}
