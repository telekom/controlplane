// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"

	"github.com/pkg/errors"
)

var ErrNotFound = errors.New("snapshot not found")

type Snapshot interface {
	ID() string
}

// SensitiveSnapshot is optionally implemented by snapshots that may contain
// confidential data. Stores must restrict access to such snapshots.
type SensitiveSnapshot interface {
	Sensitive() bool
}

func isSensitive(snap any) bool {
	s, ok := snap.(SensitiveSnapshot)
	return ok && s.Sensitive()
}

type SnapshotList[T Snapshot] interface {
	Add(snap T)
	All() []T
	New() T
}

type SnapshotStore[T Snapshot] interface {
	GetAll(ctx context.Context, id string, snapshots SnapshotList[T]) error
	GetLatest(ctx context.Context, id string, snapshot T) error
	GetVersion(ctx context.Context, id string, version int, snapshot T) error
	Set(ctx context.Context, snap T) error
	Delete(ctx context.Context, id string) error
	Clean() error
}
