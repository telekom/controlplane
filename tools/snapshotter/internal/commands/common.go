// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"errors"

	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
)

var (
	noStore bool
)

var errNoStorePath = errors.New("no store path configured: set storePath in the config file or pass --store")

// resolveStorePath returns override if set, otherwise cfg.StorePath.
// An empty result is only an error when storing is needed (noStore is false).
func resolveStorePath(cfg config.Config, override string, noStore bool) (string, error) {
	if override != "" {
		return override, nil
	}
	if cfg.StorePath == "" && !noStore {
		return "", errNoStorePath
	}
	return cfg.StorePath, nil
}

func NewStore(path string, noStore bool) store.SnapshotStore[*snapshot.Snapshot] {
	if noStore {
		return store.NewNopStore[*snapshot.Snapshot]()
	}
	return store.NewFileStore[*snapshot.Snapshot](path)
}
