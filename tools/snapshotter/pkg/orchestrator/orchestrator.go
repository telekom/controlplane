// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package orchestrator

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/obfuscator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/source"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

type Orchestrator struct {
	source             source.Source
	store              store.SnapshotStore
	obfuscationTargets []obfuscator.ObfuscationTarget
	ReportResult       func(result diffmatcher.Result, snapID string)
}

func NewOrchestrator(source source.Source, store store.SnapshotStore, obfuscationTargets []obfuscator.ObfuscationTarget) *Orchestrator {
	return &Orchestrator{
		source:             source,
		store:              store,
		obfuscationTargets: obfuscationTargets,
		ReportResult:       reportResult,
	}
}

type RunOptions struct {
	Limit        int
	ResourceType string
	Resources    []string
}

func (o *Orchestrator) Run(ctx context.Context, opts RunOptions) (snaps []snapshot.Snapshot, err error) {

	if opts.ResourceType != "" && len(opts.Resources) > 0 {
		for _, resID := range opts.Resources {
			if snap, err := o.Do(ctx, opts.ResourceType, resID); err != nil {
				return nil, err
			} else {
				snaps = append(snaps, snap)
			}
		}
		return snaps, nil
	}

	zap.L().Info("taking global snapshot", zap.String("resourceType", opts.ResourceType), zap.Int("limit", opts.Limit))

	snapMap, err := o.source.TakeGlobalSnapshot(ctx, opts.ResourceType, opts.Limit)
	if err != nil {
		return snaps, err
	}

	zap.L().Info("taken global snapshot", zap.Int("count", len(snapMap)))

	for _, snap := range snapMap {
		if err := obfuscator.Obfuscate(snap.State, o.obfuscationTargets...); err != nil {
			return nil, errors.Wrapf(err, "failed to obfuscate %q", snap.ID)
		}
		if err := o.store.Set(ctx, snap); err != nil {
			return nil, errors.Wrapf(err, "failed to store snapshot %q", snap.ID)
		}

		oldSnap, err := o.store.GetVersion(ctx, snap.ID, -1)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return nil, errors.Wrapf(err, "failed to get previous snapshot for %q", snap.ID)
			}
		}

		diffResult := diffmatcher.Compare(&oldSnap, &snap)
		o.ReportResult(diffResult, snap.ID)
		snaps = append(snaps, snap)
	}

	return snaps, nil
}

func (o *Orchestrator) Do(ctx context.Context, resourceType, resourceId string) (snapshot.Snapshot, error) {
	snap, err := o.source.TakeSnapshot(ctx, resourceType, resourceId)
	if err != nil {
		return snapshot.Snapshot{}, err
	}

	if err := obfuscator.Obfuscate(snap.State, o.obfuscationTargets...); err != nil {
		return snapshot.Snapshot{}, errors.Wrapf(err, "failed to obfuscate %q", snap.ID)
	}
	if err := o.store.Set(ctx, snap); err != nil {
		return snapshot.Snapshot{}, errors.Wrapf(err, "failed to store snapshot %q", snap.ID)
	}

	oldSnap, err := o.store.GetVersion(ctx, snap.ID, -1)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return snapshot.Snapshot{}, errors.Wrapf(err, "failed to get previous snapshot for %q", snap.ID)
		}
	}

	diffResult := diffmatcher.Compare(&oldSnap, &snap)
	o.ReportResult(diffResult, snap.ID)

	return snap, nil
}

func reportResult(result diffmatcher.Result, snapID string) {
	if result.Changed {
		// For now, just print the diff to stdout
		// In the future, this could be extended to send notifications, create tickets, etc.
		println("Snapshot changed for ID:", snapID)
	} else {
		println("No changes for snapshot ID:", snapID)
	}
}
