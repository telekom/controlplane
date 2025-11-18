// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import "context"

var _ SnapshotStore[Snapshot] = &NopStore[Snapshot]{}

type NopStore[T Snapshot] struct {
}

func NewNopStore[T Snapshot]() *NopStore[T] {
	return &NopStore[T]{}
}

// Delete implements SnapshotStore.
func (n *NopStore[T]) Delete(ctx context.Context, id string) error {
	return nil
}

// GetAll implements SnapshotStore.
func (n *NopStore[T]) GetAll(ctx context.Context, id string, snapshots SnapshotList[T]) error {
	return nil
}

// GetLatest implements SnapshotStore.
func (n *NopStore[T]) GetLatest(ctx context.Context, id string, snapshot T) error {
	return nil
}

// GetVersion implements SnapshotStore.
func (n *NopStore[T]) GetVersion(ctx context.Context, id string, version int, snapshot T) error {
	return nil
}

// List implements SnapshotStore.
func (n *NopStore[T]) List(ctx context.Context, snapshots SnapshotList[T]) error {
	return nil
}

// Set implements SnapshotStore.
func (n *NopStore[T]) Set(ctx context.Context, snap T) error {
	return nil
}

// Clean implements SnapshotStore.
func (n *NopStore[T]) Clean() error {
	return nil
}
