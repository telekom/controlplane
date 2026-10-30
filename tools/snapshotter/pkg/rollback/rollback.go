// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/goccy/go-yaml"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

// BackupDir is the directory below the store path that holds pre-apply backups.
// The leading dot keeps it distinct from snapshot IDs (<env>-<zone>-<id>).
const BackupDir = ".backups"

// Validate checks that the snapshot can safely be applied to the given environment and zone.
func Validate(snap *snapshot.Snapshot, environment, zone string) error {
	if !snap.Raw {
		return errors.New("snapshot is not raw (it may be obfuscated or decoded); take it with 'snap --raw'")
	}
	if snap.Environment != environment || snap.Zone != zone {
		return fmt.Errorf("snapshot is for environment %q zone %q, but the selected source is environment %q zone %q",
			snap.Environment, snap.Zone, environment, zone)
	}
	s := snap.State
	if s == nil || s.Route == nil || deref(s.Route.Id) == "" {
		return errors.New("snapshot does not contain a route with an ID")
	}
	if s.Service == nil || deref(s.Service.Id) == "" {
		return errors.New("snapshot does not contain a service with an ID")
	}
	if s.Upstream != nil && deref(s.Upstream.Id) == "" {
		return errors.New("snapshot contains an upstream without an ID")
	}
	for _, p := range s.Plugins {
		if deref(p.Id) == "" {
			return fmt.Errorf("snapshot contains plugin %q without an ID", deref(p.Name))
		}
	}
	for _, t := range s.Targets {
		if deref(t.Id) == "" {
			return fmt.Errorf("snapshot contains target %q without an ID", deref(t.Target))
		}
	}
	return nil
}

// LoadFile reads a snapshot YAML file.
func LoadFile(path string) (*snapshot.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot file: %w", err)
	}
	snap := &snapshot.Snapshot{}
	if err := yaml.Unmarshal(data, snap); err != nil {
		return nil, fmt.Errorf("parsing snapshot file %s: %w", path, err)
	}
	return snap, nil
}

// Prepared holds the current gateway state and the plan to reach the desired snapshot.
type Prepared struct {
	Desired *snapshot.Snapshot
	// Current is the unmodified gateway state, used for the backup.
	Current *snapshot.Snapshot
	Diff    diffmatcher.Result
	Plan    []Operation
}

// HasWrites reports whether the plan changes the gateway.
func (p *Prepared) HasWrites() bool {
	for _, op := range p.Plan {
		if op.IsWrite() {
			return true
		}
	}
	return false
}

// Rollback restores a raw route snapshot on a Kong gateway.
type Rollback struct {
	Client kong.ClientWithResponsesInterface
	Prune  bool
	Now    func() time.Time
}

// Prepare reads the current state, computes the diff (current -> desired) and the plan.
func (r *Rollback) Prepare(ctx context.Context, desired *snapshot.Snapshot) (*Prepared, error) {
	state, err := ReadState(ctx, r.Client, desired.State)
	if err != nil {
		return nil, fmt.Errorf("reading current gateway state: %w", err)
	}
	current := &snapshot.Snapshot{
		Environment: desired.Environment,
		Zone:        desired.Zone,
		Id:          desired.Id,
		Raw:         true,
		State:       state,
	}
	return &Prepared{
		Desired: desired,
		Current: current,
		Diff:    diffmatcher.Compare(withoutTimestamps(current), withoutTimestamps(desired)),
		Plan:    Plan(desired.State, state, r.Prune),
	}, nil
}

// Apply writes a backup of the current state below storePath, executes the plan and
// verifies the result. It returns the backup path.
func (r *Rollback) Apply(ctx context.Context, prepared *Prepared, storePath string) (string, error) {
	if storePath == "" {
		return "", errors.New("a store path is required for --apply to write the backup")
	}
	backupPath, err := r.writeBackup(prepared.Current, storePath)
	if err != nil {
		return "", fmt.Errorf("writing backup: %w", err)
	}
	zap.L().Info("wrote backup of current gateway state", zap.String("path", backupPath))

	if _, err := Execute(ctx, r.Client, prepared.Plan); err != nil {
		return backupPath, fmt.Errorf("applying snapshot (backup: %s): %w", backupPath, err)
	}

	if err := r.verify(ctx, prepared.Desired); err != nil {
		return backupPath, fmt.Errorf("verifying applied snapshot (backup: %s): %w", backupPath, err)
	}
	return backupPath, nil
}

func (r *Rollback) verify(ctx context.Context, desired *snapshot.Snapshot) error {
	after, err := r.Prepare(ctx, desired)
	if err != nil {
		return err
	}
	if after.HasWrites() {
		return fmt.Errorf("gateway state does not match the snapshot:\n%s", after.Diff.Text)
	}
	return nil
}

func (r *Rollback) writeBackup(current *snapshot.Snapshot, storePath string) (string, error) {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	dir := filepath.Join(storePath, BackupDir, current.ID())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, strconv.FormatInt(now().Unix(), 10)+store.Ext)
	data, err := yaml.MarshalWithOptions(current, yaml.UseLiteralStyleIfMultiline(true))
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	return path, f.Close()
}
