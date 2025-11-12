// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

var (
	noStore bool
)

func NewStore(path string, noStore bool) store.SnapshotStore[*snapshot.Snapshot] {
	if noStore {
		return store.NewNopStore[*snapshot.Snapshot]()
	}
	return store.NewFileStore[*snapshot.Snapshot](path)
}
