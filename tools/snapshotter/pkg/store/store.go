// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

var ErrNotFound = errors.New("snapshot not found")

type SnapshotStore interface {
	List(ctx context.Context) ([]snapshot.Snapshot, error)
	GetAll(ctx context.Context, id string) ([]snapshot.Snapshot, error)
	GetLatest(ctx context.Context, id string) (snapshot.Snapshot, error)
	GetVersion(ctx context.Context, id string, version int) (snapshot.Snapshot, error)
	Set(ctx context.Context, snap snapshot.Snapshot) error
	Delete(ctx context.Context, id string) error
}
