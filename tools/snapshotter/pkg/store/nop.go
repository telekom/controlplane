// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

var _ SnapshotStore = &NopStore{}

type NopStore struct {
}

// GetAll implements SnapshotStore.
func (n *NopStore) GetAll(ctx context.Context, id string) ([]snapshot.Snapshot, error) {
	panic("unimplemented")
}

// GetLatest implements SnapshotStore.
func (n *NopStore) GetLatest(ctx context.Context, id string) (snapshot.Snapshot, error) {
	panic("unimplemented")
}

// List implements SnapshotStore.
func (n *NopStore) List(ctx context.Context) ([]snapshot.Snapshot, error) {
	panic("unimplemented")
}

// Delete implements SnapshotStore.
func (n *NopStore) Delete(ctx context.Context, id string) error {
	return nil
}

// GetVersion implements SnapshotStore.
func (n *NopStore) GetVersion(ctx context.Context, id string, version int) (snapshot.Snapshot, error) {
	return snapshot.Snapshot{}, ErrNotFound
}

// Set implements SnapshotStore.
func (n *NopStore) Set(ctx context.Context, snap snapshot.Snapshot) error {
	return nil
}
