// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

const (
	Ext = ".snap.yaml"
)

var _ SnapshotStore = &FileStore{}

type FileStore struct {
	FilePath string
}

func NewFileStore(filePath string) *FileStore {
	_ = os.MkdirAll(filePath, 0o755)
	return &FileStore{
		FilePath: filePath,
	}
}

func (f *FileStore) makeFilePath(id string, version int) string {
	return filepath.Join(f.FilePath, id, strconv.Itoa(version)+Ext)
}

// Delete implements SnapshotStore.
func (f *FileStore) Delete(ctx context.Context, id string) error {
	path := f.makeFilePath(id, 0)
	dir := filepath.Dir(path)
	return os.RemoveAll(dir)
}

func (f *FileStore) getAvailableVersions(ctx context.Context, id string) ([]int, error) {
	path := filepath.Join(f.FilePath, id)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, ErrNotFound
	}

	var versions []int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !f.isSnapshotFile(entry) {
			continue
		}
		versionStr := strings.TrimSuffix(entry.Name(), Ext)
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))
	return versions, nil
}

// GetAll implements SnapshotStore.
func (f *FileStore) GetAll(ctx context.Context, id string) ([]snapshot.Snapshot, error) {
	versions, err := f.getAvailableVersions(ctx, id)
	if err != nil {
		return nil, err
	}

	var snaps []snapshot.Snapshot
	for _, version := range versions {
		snap, err := f.GetVersion(ctx, id, version)
		if err != nil {
			return nil, err
		}
		snaps = append(snaps, snap)
	}
	return snaps, nil
}

// GetLatest implements SnapshotStore.
func (f *FileStore) GetLatest(ctx context.Context, id string) (snapshot.Snapshot, error) {
	versions, err := f.getAvailableVersions(ctx, id)
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	if len(versions) == 0 {
		return snapshot.Snapshot{}, fmt.Errorf("no versions found for id %s", id)
	}
	latestVersion := versions[0]
	return f.GetVersion(ctx, id, latestVersion)
}

// GetVersion implements SnapshotStore.
// version == 0 --> latest version
// version == -1 --> version before latest
func (f *FileStore) GetVersion(ctx context.Context, id string, version int) (snapshot.Snapshot, error) {
	if version == -1 {
		versions, err := f.getAvailableVersions(ctx, id)
		if err != nil {
			return snapshot.Snapshot{}, err
		}
		if len(versions) < 2 {
			return snapshot.Snapshot{}, ErrNotFound
		}
		version = versions[1]
	}

	if version == 0 {
		return f.GetLatest(ctx, id)
	}

	path := f.makeFilePath(id, version)
	data, err := os.ReadFile(path)
	if err != nil {
		return snapshot.Snapshot{}, err
	}

	snap, err := snapshot.Unmarshal(data)
	if err != nil {
		return snapshot.Snapshot{}, err
	}

	return *snap, nil
}

// List implements SnapshotStore.
func (f *FileStore) List(ctx context.Context) ([]snapshot.Snapshot, error) {
	entries, err := os.ReadDir(f.FilePath)
	if err != nil {
		return nil, err
	}

	var snaps []snapshot.Snapshot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		snapList, err := f.GetAll(ctx, id)
		if err != nil {
			return nil, err
		}
		snaps = append(snaps, snapList...)
	}
	return snaps, nil
}

// Set implements SnapshotStore.
func (f *FileStore) Set(ctx context.Context, snap snapshot.Snapshot) error {
	versions, err := f.getAvailableVersions(ctx, snap.Path())
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		err = os.MkdirAll(filepath.Dir(f.makeFilePath(snap.Path(), 0)), 0o755)
		if err != nil {
			return err
		}
	}

	var newVersion int
	if len(versions) == 0 {
		newVersion = 1
	} else {
		newVersion = versions[0] + 1
	}

	path := f.makeFilePath(snap.Path(), newVersion)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(snap)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	return nil
}

func (f *FileStore) Clean() error {
	if err := os.RemoveAll(f.FilePath); err != nil {
		return err
	}
	return os.MkdirAll(f.FilePath, 0o755)
}

func (f *FileStore) isSnapshotFile(file os.DirEntry) bool {
	if file.IsDir() {
		return false
	}
	if !strings.HasSuffix(file.Name(), Ext) {
		return false
	}
	return true
}
