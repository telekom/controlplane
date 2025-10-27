// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package orchestrator

import (
	"context"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/config"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/decoder"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/diffmatcher"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/obfuscator"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/source"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/store"
	"go.uber.org/zap"
)

type Orchestrator struct {
	Environment        string
	Zone               string
	source             source.Source
	store              store.SnapshotStore[*snapshot.Snapshot]
	obfuscationTargets []obfuscator.ObfuscationTarget
	decoderTargets     []decoder.DecoderTarget
	ReportResult       func(result diffmatcher.Result, snapID string)
}

func NewFromConfig(cfg config.Config, store store.SnapshotStore[*snapshot.Snapshot]) map[string]*Orchestrator {
	instances := map[string]*Orchestrator{}

	for key, sourceCfg := range cfg.GetSourceConfigs() {
		zap.L().Info("setting up source", zap.String("key", key))
		kongSource, err := source.NewKongSourceFromConfig(sourceCfg, sourceCfg.Tags)
		if err != nil {
			panic(err)
		}
		instances[key] = NewOrchestrator(kongSource, store)
		instances[key].Environment = sourceCfg.Environment
		instances[key].Zone = sourceCfg.Zone
		instances[key].SetObfuscationTargets(sourceCfg.Obfuscators...)
		instances[key].SetDecoderTargets(sourceCfg.Decoders...)
	}

	return instances
}

func NewOrchestrator(source source.Source, store store.SnapshotStore[*snapshot.Snapshot]) *Orchestrator {
	return &Orchestrator{
		source: source,
		store:  store,
		ReportResult: func(result diffmatcher.Result, snapID string) {
			// noop
		},
	}
}

type RunOptions struct {
	Limit        int
	ResourceType string
	Resources    []string
}

func (o *Orchestrator) MakeId(resourceId string) string {
	return snapshot.MakePath(o.Environment, o.Zone, resourceId)
}

func (o *Orchestrator) SetObfuscationTargets(targets ...obfuscator.ObfuscationTarget) {
	o.obfuscationTargets = targets
}
func (o *Orchestrator) SetDecoderTargets(targets ...decoder.DecoderTarget) {
	o.decoderTargets = targets
}

func (o *Orchestrator) Run(ctx context.Context, opts RunOptions) (snaps []*snapshot.Snapshot, err error) {

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

	ch := make(chan *snapshot.Snapshot, 10)
	err = o.source.TakeGlobalSnapshot(ctx, opts.ResourceType, opts.Limit, ch)

	for {
		select {
		case snap, ok := <-ch:
			if !ok {
				return snaps, nil
			}
			if err := o.handleSnapshot(ctx, snap); err != nil {
				return snaps, err
			}
			snaps = append(snaps, snap)
		case <-ctx.Done():
			return snaps, ctx.Err()
		}
	}
}

func (o *Orchestrator) handleSnapshot(ctx context.Context, snap *snapshot.Snapshot) error {
	snap.Environment = o.Environment
	snap.Zone = o.Zone
	zap.L().Info("handling snapshot", zap.String("id", snap.ID()))

	if err := obfuscator.Obfuscate(snap.State, o.obfuscationTargets...); err != nil {
		return errors.Wrapf(err, "failed to obfuscate %q", snap.ID())
	}

	if err := decoder.Decode(snap.State, o.decoderTargets); err != nil {
		return errors.Wrapf(err, "failed to decode %q", snap.ID())
	}

	oldSnap := &snapshot.Snapshot{}
	err := o.store.GetVersion(ctx, snap.ID(), 0, oldSnap)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return errors.Wrapf(err, "failed to get previous snapshot for %q", snap.ID())
		}
	}

	diffResult := diffmatcher.Compare(oldSnap, snap)
	if diffResult.Changed {
		if err := o.store.Set(ctx, snap); err != nil {
			return errors.Wrapf(err, "failed to store snapshot %q", snap.ID())
		}
	}
	o.ReportResult(diffResult, snap.ID())
	return nil
}

func (o *Orchestrator) Do(ctx context.Context, resourceType, resourceId string) (*snapshot.Snapshot, error) {
	snap, err := o.source.TakeSnapshot(ctx, resourceType, resourceId)
	if err != nil {
		return nil, err
	}
	return snap, o.handleSnapshot(ctx, snap)
}
