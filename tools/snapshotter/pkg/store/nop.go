// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import "context"

var _ SnapshotStore[Snapshot] = &NopStore{}

type NopStore struct {
}

// Delete implements SnapshotStore.
func (n *NopStore) Delete(ctx context.Context, id string) error {
	return nil
}

// GetAll implements SnapshotStore.
func (n *NopStore) GetAll(ctx context.Context, id string, snapshots SnapshotList[Snapshot]) error {
	return nil
}

// GetLatest implements SnapshotStore.
func (n *NopStore) GetLatest(ctx context.Context, id string, snapshot Snapshot) error {
	return nil
}

// GetVersion implements SnapshotStore.
func (n *NopStore) GetVersion(ctx context.Context, id string, version int, snapshot Snapshot) error {
	return nil
}

// List implements SnapshotStore.
func (n *NopStore) List(ctx context.Context, snapshots SnapshotList[Snapshot]) error {
	return nil
}

// Set implements SnapshotStore.
func (n *NopStore) Set(ctx context.Context, snap Snapshot) error {
	return nil
}
