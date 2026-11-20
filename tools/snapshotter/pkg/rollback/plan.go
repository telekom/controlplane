// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"cmp"
	"fmt"
	"slices"

	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/tools/snapshotter/pkg/snapshot"
)

type Action string

const (
	ActionUpsert Action = "upsert"
	ActionDelete Action = "delete"
	// ActionKeep marks an entity that only exists on the gateway and is kept because pruning is disabled.
	ActionKeep Action = "keep"
)

type EntityKind string

const (
	KindUpstream EntityKind = "upstream"
	KindTarget   EntityKind = "target"
	KindService  EntityKind = "service"
	KindRoute    EntityKind = "route"
	KindPlugin   EntityKind = "plugin"
)

// Operation is a single planned change on the gateway.
type Operation struct {
	Action Action     `json:"action" yaml:"action"`
	Kind   EntityKind `json:"kind" yaml:"kind"`
	ID     string     `json:"id" yaml:"id"`
	// Name is the plugin name (plugins only).
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// ParentID is the upstream ID (targets only).
	ParentID string `json:"parentId,omitempty" yaml:"parentId,omitempty"`
	// Entity is the desired entity for upserts.
	Entity any `json:"-" yaml:"-"`
}

func (o Operation) String() string {
	subject := string(o.Kind)
	if o.Name != "" {
		subject += " " + o.Name
	}
	subject += " " + o.ID
	switch o.Action {
	case ActionDelete:
		return fmt.Sprintf("delete %s (prune)", subject)
	case ActionKeep:
		return fmt.Sprintf("keep %s: would be kept (use --prune to delete)", subject)
	default:
		return fmt.Sprintf("%s %s", o.Action, subject)
	}
}

// IsWrite reports whether the operation changes the gateway.
func (o Operation) IsWrite() bool {
	return o.Action == ActionUpsert || o.Action == ActionDelete
}

// Plan computes the ordered operations that turn current into desired:
// upstream, targets, service, route, plugins, then deletes/keeps of gateway-only entities.
// Entities that are equal apart from timestamps are skipped.
func Plan(desired, current *snapshot.State, prune bool) []Operation {
	if current == nil {
		current = &snapshot.State{}
	}
	var ops []Operation
	var extra []Operation

	if desired.Upstream != nil {
		if current.Upstream == nil || !equalIgnoringTimestamps(desired.Upstream, current.Upstream) {
			ops = append(ops, Operation{Action: ActionUpsert, Kind: KindUpstream, ID: deref(desired.Upstream.Id), Entity: desired.Upstream})
		}
	} else if current.Upstream != nil {
		extra = append(extra, Operation{Kind: KindUpstream, ID: deref(current.Upstream.Id)})
	}

	if desired.Upstream != nil {
		upstreamID := deref(desired.Upstream.Id)
		currentTargets := map[string]kong.Target{}
		for _, t := range current.Targets {
			currentTargets[deref(t.Id)] = t
		}
		desiredIDs := map[string]bool{}
		for i := range desired.Targets {
			t := desired.Targets[i]
			id := deref(t.Id)
			desiredIDs[id] = true
			if cur, ok := currentTargets[id]; ok && equalIgnoringTimestamps(t, cur) {
				continue
			}
			ops = append(ops, Operation{Action: ActionUpsert, Kind: KindTarget, ID: id, ParentID: upstreamID, Entity: &desired.Targets[i]})
		}
		var extraTargets []Operation
		for id := range currentTargets {
			if !desiredIDs[id] {
				extraTargets = append(extraTargets, Operation{Kind: KindTarget, ID: id, ParentID: deref(current.Upstream.Id)})
			}
		}
		slices.SortFunc(extraTargets, func(a, b Operation) int { return cmp.Compare(a.ID, b.ID) })
		extra = append(extra, extraTargets...)
	}

	if desired.Service != nil && (current.Service == nil || !equalIgnoringTimestamps(desired.Service, current.Service)) {
		ops = append(ops, Operation{Action: ActionUpsert, Kind: KindService, ID: deref(desired.Service.Id), Entity: desired.Service})
	}
	if desired.Route != nil && (current.Route == nil || !equalIgnoringTimestamps(desired.Route, current.Route)) {
		ops = append(ops, Operation{Action: ActionUpsert, Kind: KindRoute, ID: deref(desired.Route.Id), Entity: desired.Route})
	}

	currentPlugins := map[string]kong.Plugin{}
	for _, p := range current.Plugins {
		currentPlugins[deref(p.Id)] = p
	}
	desiredPluginIDs := map[string]bool{}
	for i := range desired.Plugins {
		p := desired.Plugins[i]
		id := deref(p.Id)
		desiredPluginIDs[id] = true
		if cur, ok := currentPlugins[id]; ok && equalIgnoringTimestamps(p, cur) {
			continue
		}
		ops = append(ops, Operation{Action: ActionUpsert, Kind: KindPlugin, ID: id, Name: deref(p.Name), Entity: &desired.Plugins[i]})
	}
	var extraPlugins []Operation
	for id, p := range currentPlugins {
		if !desiredPluginIDs[id] {
			extraPlugins = append(extraPlugins, Operation{Kind: KindPlugin, ID: id, Name: deref(p.Name)})
		}
	}
	slices.SortFunc(extraPlugins, func(a, b Operation) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	extra = append(extra, extraPlugins...)

	// Plugins and targets reference their parents, so delete them before the upstream.
	slices.SortStableFunc(extra, func(a, b Operation) int {
		return cmp.Compare(deleteRank(a.Kind), deleteRank(b.Kind))
	})
	for _, op := range extra {
		op.Action = ActionKeep
		if prune {
			op.Action = ActionDelete
		}
		ops = append(ops, op)
	}
	return ops
}

func deleteRank(k EntityKind) int {
	switch k {
	case KindPlugin:
		return 0
	case KindTarget:
		return 1
	default:
		return 2
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
