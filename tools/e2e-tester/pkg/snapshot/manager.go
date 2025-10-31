// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package snapshot

import (
	"context"
	"path/filepath"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

// SnapshotList implements the store.SnapshotList interface for CommandSnapshots
type SnapshotList struct {
	snapshots []*CommandSnapshot
}

// Add adds a snapshot to the list
func (l *SnapshotList) Add(snap *CommandSnapshot) {
	l.snapshots = append(l.snapshots, snap)
}

// All returns all snapshots in the list
func (l *SnapshotList) All() []*CommandSnapshot {
	return l.snapshots
}

// New creates a new empty snapshot
func (l *SnapshotList) New() *CommandSnapshot {
	return &CommandSnapshot{}
}

type SnapshotManager interface {
	StoreSnapshot(ctx context.Context, snap *CommandSnapshot) error
	GetLatestSnapshot(ctx context.Context, id string) (*CommandSnapshot, error)
	GetAllSnapshots(ctx context.Context, id string) ([]*CommandSnapshot, error)
	CompareSnapshots(expected, actual *CommandSnapshot) *ComparisonResult
	GetSnapshotPath(suite, env, caseIndex, caseName string) string
}

var _ SnapshotManager = (*Manager)(nil)

// Manager handles storage and comparison of command snapshots
type Manager struct {
	snapshotStore store.SnapshotStore[*CommandSnapshot]
	baseDir       string
}

// NewManager creates a new snapshot manager
func NewManager(baseDir string) *Manager {
	fileStore := store.NewFileStore[*CommandSnapshot](baseDir)
	return &Manager{
		snapshotStore: fileStore,
		baseDir:       baseDir,
	}
}

// StoreSnapshot stores a command snapshot
func (m *Manager) StoreSnapshot(ctx context.Context, snap *CommandSnapshot) error {
	return m.snapshotStore.Set(ctx, snap)
}

// GetLatestSnapshot retrieves the latest snapshot for a given command
func (m *Manager) GetLatestSnapshot(ctx context.Context, id string) (*CommandSnapshot, error) {
	snap := &CommandSnapshot{}
	err := m.snapshotStore.GetLatest(ctx, id, snap)
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// GetAllSnapshots retrieves all snapshots for a given command
func (m *Manager) GetAllSnapshots(ctx context.Context, id string) ([]*CommandSnapshot, error) {
	list := &SnapshotList{}
	err := m.snapshotStore.GetAll(ctx, id, list)
	if err != nil {
		return nil, err
	}
	return list.All(), nil
}

// ComparisonResult contains the result of a snapshot comparison
type ComparisonResult struct {
	HasChanges       bool
	NumberOfChanges  int
	DiffText         string
	ExpectedSnapshot *CommandSnapshot
	ActualSnapshot   *CommandSnapshot
}

// CompareSnapshots compares two command snapshots and returns the differences
func (m *Manager) CompareSnapshots(expected, actual *CommandSnapshot) *ComparisonResult {

	zap.L().Debug("Comparing snapshots", zap.String("spected", expected.ID()), zap.String("actual", actual.ID()))
	// Use the diffmatcher to compare snapshots
	result := diffmatcher.Compare(expected, actual)

	return &ComparisonResult{
		HasChanges:       result.Changed,
		NumberOfChanges:  result.NumberOfChanges,
		DiffText:         result.Text,
		ExpectedSnapshot: expected,
		ActualSnapshot:   actual,
	}
}

// GetSnapshotPath returns the full path to a snapshot file
func (m *Manager) GetSnapshotPath(suite, env, caseIndex, caseName string) string {
	id := MakeSnapshotID(suite, env, caseIndex, caseName)
	return filepath.Join(m.baseDir, id)
}
