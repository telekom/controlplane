// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManager_StoreAndRetrieve(t *testing.T) {
	// Create a temporary directory for snapshots
	tempDir, err := os.MkdirTemp("", "e2e-tester-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create snapshot manager
	manager := NewManager(tempDir)

	// Create a test snapshot
	testSnapshot := &CommandSnapshot{
		Id:      "test-snapshot",
		Version: 1,
		Output: CommandOutput{
			Command:     "test command",
			ExitCode:    0,
			Stdout:      "Test stdout",
			Stderr:      "Test stderr",
			Environment: "test-env",
			Duration:    "1s",
		},
	}

	// Store the snapshot
	ctx := context.Background()
	err = manager.StoreSnapshot(ctx, testSnapshot)
	if err != nil {
		t.Fatalf("Failed to store snapshot: %v", err)
	}

	// Retrieve the snapshot
	retrievedSnapshot, err := manager.GetLatestSnapshot(ctx, testSnapshot.ID())
	if err != nil {
		t.Fatalf("Failed to retrieve snapshot: %v", err)
	}

	// Verify retrieved snapshot
	if retrievedSnapshot.Id != testSnapshot.Id {
		t.Errorf("Expected ID to be '%s', but got '%s'", testSnapshot.Id, retrievedSnapshot.Id)
	}
	if retrievedSnapshot.Output.Command != testSnapshot.Output.Command {
		t.Errorf("Expected command to be '%s', but got '%s'", testSnapshot.Output.Command, retrievedSnapshot.Output.Command)
	}
	if retrievedSnapshot.Output.ExitCode != testSnapshot.Output.ExitCode {
		t.Errorf("Expected exit code to be %d, but got %d", testSnapshot.Output.ExitCode, retrievedSnapshot.Output.ExitCode)
	}
}

func TestManager_CompareSnapshots(t *testing.T) {
	// Create a temporary directory for snapshots
	tempDir, err := os.MkdirTemp("", "e2e-tester-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after test

	// Create snapshot manager
	manager := NewManager(tempDir)

	// Create test snapshots
	snapshot1 := &CommandSnapshot{
		Id:      "test-snapshot-1",
		Version: 1,
		Output: CommandOutput{
			Command:     "test command",
			ExitCode:    0,
			Stdout:      "Same stdout",
			Stderr:      "Same stderr",
			Environment: "test-env",
			Duration:    "1s",
		},
	}

	snapshot2 := &CommandSnapshot{
		Id:      "test-snapshot-2",
		Version: 1,
		Output: CommandOutput{
			Command:     "test command",
			ExitCode:    0,
			Stdout:      "Same stdout",
			Stderr:      "Same stderr",
			Environment: "test-env",
			Duration:    "1s",
		},
	}

	snapshot3 := &CommandSnapshot{
		Id:      "test-snapshot-3",
		Version: 1,
		Output: CommandOutput{
			Command:     "test command",
			ExitCode:    0,
			Stdout:      "Different stdout",
			Stderr:      "Different stderr",
			Environment: "test-env",
			Duration:    "1s",
		},
	}

	// Compare identical snapshots
	result1 := manager.CompareSnapshots(snapshot1, snapshot2)
	if result1.HasChanges {
		t.Errorf("Expected no changes between identical snapshots")
	}

	// Compare different snapshots
	result2 := manager.CompareSnapshots(snapshot1, snapshot3)
	if !result2.HasChanges {
		t.Errorf("Expected changes between different snapshots")
	}
}

func TestGetSnapshotPath(t *testing.T) {
	manager := NewManager("/test/path")
	suite := "test-suite"
	env := "test-env"
	caseIndex := "0"
	caseName := "test-case"

	expectedPath := filepath.Join("/test/path", MakeSnapshotID(suite, env, caseIndex, caseName))
	actualPath := manager.GetSnapshotPath(suite, env, caseIndex, caseName)

	if actualPath != expectedPath {
		t.Errorf("Expected path '%s', but got '%s'", expectedPath, actualPath)
	}
}
