// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"go.uber.org/zap"
)

const (
	Ext = ".snap.yaml"
)

var _ SnapshotStore[Snapshot] = &FileStore[Snapshot]{}

type FileStore[T Snapshot] struct {
	FilePath    string
	MaxVersions int
}

func NewFileStore[T Snapshot](filePath string) *FileStore[T] {
	_ = os.MkdirAll(filePath, 0o755)
	return &FileStore[T]{
		FilePath:    filePath,
		MaxVersions: 1,
	}
}

func (f *FileStore[T]) makeFilePath(id string, version int) string {
	return filepath.Join(f.FilePath, id, strconv.Itoa(version)+Ext)
}

// Delete implements SnapshotStore.
func (f *FileStore[T]) Delete(ctx context.Context, id string) error {
	path := f.makeFilePath(id, 0)
	dir := filepath.Dir(path)
	return os.RemoveAll(dir)
}

func (f *FileStore[T]) DeleteVersion(ctx context.Context, id string, version int) error {
	path := f.makeFilePath(id, version)
	return os.Remove(path)
}

func (f *FileStore[T]) getAvailableVersions(ctx context.Context, id string) ([]int, error) {
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
func (f *FileStore[T]) GetAll(ctx context.Context, id string, snaps SnapshotList[T]) error {
	versions, err := f.getAvailableVersions(ctx, id)
	if err != nil {
		return err
	}

	for _, version := range versions {
		snap := snaps.New()
		err := f.GetVersion(ctx, id, version, snap)
		if err != nil {
			return err
		}
		snaps.Add(snap)
	}
	return nil
}

// GetLatest implements SnapshotStore.
func (f *FileStore[T]) GetLatest(ctx context.Context, id string, snap T) error {
	versions, err := f.getAvailableVersions(ctx, id)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return ErrNotFound
	}
	latestVersion := versions[0]
	return f.GetVersion(ctx, id, latestVersion, snap)
}

// GetVersion implements SnapshotStore.
// version == 0 --> latest version
// version == -1 --> version before latest
func (f *FileStore[T]) GetVersion(ctx context.Context, id string, version int, snap T) error {
	if version == -1 {
		versions, err := f.getAvailableVersions(ctx, id)
		if err != nil {
			return err
		}
		if len(versions) < 2 {
			return ErrNotFound
		}
		version = versions[1]
	}

	if version == 0 {
		return f.GetLatest(ctx, id, snap)
	}

	path := f.makeFilePath(id, version)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, snap)
	if err != nil {
		return err
	}

	return nil
}

// Set implements SnapshotStore.
func (f *FileStore[T]) Set(ctx context.Context, snap T) error {
	versions, err := f.getAvailableVersions(ctx, snap.ID())
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		err = os.MkdirAll(filepath.Dir(f.makeFilePath(snap.ID(), 0)), 0o755)
		if err != nil {
			return err
		}
	}

	if f.MaxVersions > 0 && len(versions) >= f.MaxVersions {
		// Delete oldest versions
		toDelete := versions[f.MaxVersions-1:]
		for _, v := range toDelete {
			if err := f.DeleteVersion(ctx, snap.ID(), v); err != nil {
				return err
			}
		}
		zap.L().Debug("cleanup versions", zap.Int("before", len(versions)), zap.Int("after", f.MaxVersions-1), zap.String("id", snap.ID()))
		versions = versions[:f.MaxVersions-1]
	}
	var newVersion int
	if len(versions) == 0 {
		newVersion = 1
	} else {
		newVersion = versions[0] + 1
	}

	path := f.makeFilePath(snap.ID(), newVersion)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := yaml.MarshalWithOptions(snap, yaml.UseLiteralStyleIfMultiline(true))
	if err != nil {
		return err
	}
	zap.L().Debug("storing snapshot", zap.String("id", snap.ID()), zap.Int("version", newVersion), zap.Int("size", len(data)))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	return nil
}

func (f *FileStore[T]) Clean() error {
	if err := os.RemoveAll(f.FilePath); err != nil {
		return err
	}
	return os.MkdirAll(f.FilePath, 0o755)
}

func (f *FileStore[T]) isSnapshotFile(file os.DirEntry) bool {
	if file.IsDir() {
		return false
	}
	if !strings.HasSuffix(file.Name(), Ext) {
		return false
	}
	return true
}
