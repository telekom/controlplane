// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/gateway/pkg/kong/naming"
)

// Origin tells which discovery step found an entity.
type Origin string

const (
	OriginNone         Origin = ""
	OriginStoredID     Origin = "storedId"
	OriginParent       Origin = "parent"
	OriginName         Origin = "name"
	OriginTags         Origin = "tags"
	OriginPreMigration Origin = "preMigration"
	OriginLegacy       Origin = "legacy"
)

// locator is one step of the discovery chain. A nil entity means "try the next step".
type locator[Resp any] func(ctx context.Context) (*Resp, Origin, error)

// firstOf runs the steps in order and returns the first entity found.
func firstOf[Resp any](ctx context.Context, steps ...locator[Resp]) (*Resp, Origin, error) {
	for _, step := range steps {
		current, origin, err := step(ctx)
		if err != nil || current != nil {
			return current, origin, err
		}
	}
	return nil, OriginNone, nil
}

// namedOwnership is a name under which the entity may exist and the predicate that proves
// an entity under that name is ours.
type namedOwnership struct {
	name   string
	owned  ownership
	origin Origin
}

// discovery finds the Kong entity of one kind that belongs to a realm route:
// byID → byParent → byName(P--R) → byTags(env,route) → byName(R, CP-owned) → byCandidates(owned).
type discovery[Req, Resp any] struct {
	api      KongAdminApi
	kind     kind[Req, Resp]
	storedID string
	parentID string
	names    naming.Names
	env      string
	route    string
	// legacy enables the Java candidates.
	legacy bool
	// policy decides what happens to foreign, ambiguous, duplicate and shared entities.
	policy discoveryPolicy
	// sharedWith returns another Kong entity that uses the found entity, or "". It guards
	// the rename of an entity found through its parent or a legacy name.
	sharedWith func(ctx context.Context, found *Resp) (string, error)
	// byNameCache holds the entities looked up by name, nil if not found.
	byNameCache map[string]*Resp
}

// discoveryPolicy decides how discovery treats entities it may not use. A nil error from a
// policy method means "skip the entity and go on".
type discoveryPolicy interface {
	foreign(ctx context.Context, kind, name, id, route string) error
	ambiguous(ctx context.Context, kind string, count int, tags []string) error
	duplicate(ctx context.Context, kind, foundName, foundID, otherName, otherID, route string) error
	shared(ctx context.Context, kind, name, id, user, route string) error
	taken(ctx context.Context, kind, name, id string, tags []string, foundName, route string) error
}

// reconcilePolicy blocks on everything that is not clearly ours. It is the default.
type reconcilePolicy struct{}

func (reconcilePolicy) foreign(_ context.Context, kind, name, id, route string) error {
	return ctrlerrors.BlockedErrorf("%s %q (%s) exists in Kong but is not owned by route %q", kind, name, id, route)
}

func (reconcilePolicy) ambiguous(_ context.Context, kind string, count int, tags []string) error {
	return ctrlerrors.BlockedErrorf("found %d %s entities with tags %v, expected at most one", count, kind, tags)
}

func (reconcilePolicy) duplicate(_ context.Context, kind, foundName, foundID, otherName, otherID, route string) error {
	return ctrlerrors.BlockedErrorf("%s %q (%s) and %q (%s) both belong to route %q; resolve the duplicate manually",
		kind, foundName, foundID, otherName, otherID, route)
}

func (reconcilePolicy) shared(_ context.Context, kind, name, id, user, route string) error {
	return ctrlerrors.BlockedErrorf("%s %q (%s) of route %q is also used by %s; resolve the sharing manually", kind, name, id, route, user)
}

func (reconcilePolicy) taken(_ context.Context, kind, name, id string, tags []string, foundName, _ string) error {
	return ctrlerrors.BlockedErrorf("%s %q (%s) is already used by env %q, route %q; cannot rename %q",
		kind, name, id, tagValue(tags, "env"), tagValue(tags, parentRoute), foundName)
}

// deletePolicy never blocks: entities that are not ours or not unambiguous are skipped and
// logged, so that a delete only removes the one entity found by the earliest step.
type deletePolicy struct{}

func (deletePolicy) foreign(ctx context.Context, kind, name, id, _ string) error {
	logr.FromContextOrDiscard(ctx).Info("Skipping kong entity not owned by route on delete", "kind", kind, "name", name, "id", id)
	return nil
}

func (deletePolicy) ambiguous(ctx context.Context, kind string, count int, tags []string) error {
	logr.FromContextOrDiscard(ctx).Info("Skipping ambiguous kong entities on delete", "kind", kind, "count", count, "tags", tags)
	return nil
}

func (deletePolicy) duplicate(ctx context.Context, kind, foundName, foundID, otherName, otherID, _ string) error {
	logr.FromContextOrDiscard(ctx).Info("Skipping duplicate kong entity on delete", "kind", kind,
		"name", foundName, "id", foundID, "duplicateName", otherName, "duplicateId", otherID)
	return nil
}

func (deletePolicy) shared(ctx context.Context, kind, name, id, user, _ string) error {
	logr.FromContextOrDiscard(ctx).Info("Skipping shared kong entity on delete", "kind", kind, "name", name, "id", id, "usedBy", user)
	return nil
}

func (deletePolicy) taken(ctx context.Context, kind, name, id string, _ []string, foundName, _ string) error {
	logr.FromContextOrDiscard(ctx).Info("Skipping foreign kong entity under the desired name on delete", "kind", kind, "name", name, "id", id, "foundName", foundName)
	return nil
}

// tagValue returns the value of the first key--value tag, or "".
func tagValue(tags []string, key string) string {
	for _, tag := range tags {
		if value, ok := strings.CutPrefix(tag, BuildTag(key, "")); ok {
			return value
		}
	}
	return ""
}

// getByName looks an entity up by name once per discovery; later calls reuse the result.
func (d *discovery[Req, Resp]) getByName(ctx context.Context, name string) (*Resp, error) {
	if current, ok := d.byNameCache[name]; ok {
		return current, nil
	}
	current, err := d.kind.get(ctx, d.api, name)
	if err != nil {
		return nil, err
	}
	if d.byNameCache == nil {
		d.byNameCache = map[string]*Resp{}
	}
	d.byNameCache[name] = current
	return current, nil
}

func (d *discovery[Req, Resp]) policyOrDefault() discoveryPolicy {
	if d.policy == nil {
		return reconcilePolicy{}
	}
	return d.policy
}

// byName lists the names in lookup order: new name, pre-migration name, Java candidates.
func (d *discovery[Req, Resp]) byName() []namedOwnership {
	cp := cpTags(d.env, d.route)
	preMigration := cp
	if d.legacy {
		preMigration = anyOf(cp, anyTag(d.names.PreMigrationOwnerTags))
	}
	named := []namedOwnership{
		{name: d.names.Desired, owned: notClaimedElsewhere(d.env, d.route), origin: OriginName},
		{name: d.names.PreMigration, owned: preMigration, origin: OriginPreMigration},
	}
	if d.legacy {
		for _, candidate := range d.names.Candidates {
			named = append(named, namedOwnership{name: candidate.Name, owned: anyOf(hasTag(candidate.OwnerTag), cp), origin: OriginLegacy})
		}
	}
	return named
}

// locate runs the chain and the conflict guard. It returns nil if nothing was found.
func (d *discovery[Req, Resp]) locate(ctx context.Context) (*Resp, Origin, error) {
	named := d.byName()
	steps := make([]locator[Resp], 0, 3+len(named))
	steps = append(steps,
		d.lookupID(d.storedID, OriginStoredID),
		d.lookupID(d.parentID, OriginParent),
		d.lookupName(named[0]),
		d.lookupTags(),
	)
	for _, n := range named[1:] {
		steps = append(steps, d.lookupName(n))
	}

	current, origin, err := firstOf(ctx, steps...)
	if err != nil || current == nil {
		return nil, origin, err
	}
	logr.FromContextOrDiscard(ctx).V(1).Info("Found kong entity", "entity", d.kind.name,
		"id", d.kind.id(current), "kongName", d.kind.entName(current), "origin", origin)

	if d.kind.entName(current) == d.names.Desired {
		return current, origin, nil
	}
	if err := d.checkConflicts(ctx, current, named); err != nil {
		return nil, origin, err
	}
	return d.checkShared(ctx, current, origin)
}

// checkShared asks the policy what to do with an entity found through its parent or a legacy
// name that another Kong entity uses as well. Skipping it means nothing was found.
func (d *discovery[Req, Resp]) checkShared(ctx context.Context, current *Resp, origin Origin) (*Resp, Origin, error) {
	if d.sharedWith == nil || (origin != OriginParent && origin != OriginPreMigration && origin != OriginLegacy) {
		return current, origin, nil
	}
	user, err := d.sharedWith(ctx, current)
	if err != nil {
		return nil, origin, fmt.Errorf("checking whether %s is shared: %w", d.kind.name, err)
	}
	if user == "" {
		return current, origin, nil
	}
	if err := d.policyOrDefault().shared(ctx, d.kind.name, d.kind.entName(current), d.kind.id(current), user, d.route); err != nil {
		return nil, origin, err
	}
	return nil, OriginNone, nil
}

func (d *discovery[Req, Resp]) lookupID(id string, origin Origin) locator[Resp] {
	return func(ctx context.Context) (*Resp, Origin, error) {
		if id == "" {
			return nil, OriginNone, nil
		}
		logr.FromContextOrDiscard(ctx).V(1).Info("Looking up kong entity by id", "entity", d.kind.name, "id", id, "origin", origin)
		current, err := d.kind.get(ctx, d.api, id)
		if err != nil {
			return nil, OriginNone, fmt.Errorf("looking up %s by id: %w", d.kind.name, err)
		}
		return current, origin, nil
	}
}

func (d *discovery[Req, Resp]) lookupName(n namedOwnership) locator[Resp] {
	return func(ctx context.Context) (*Resp, Origin, error) {
		logr.FromContextOrDiscard(ctx).V(1).Info("Looking up kong entity by name", "entity", d.kind.name, "kongName", n.name, "origin", n.origin)
		current, err := d.getByName(ctx, n.name)
		if err != nil {
			return nil, OriginNone, fmt.Errorf("looking up %s by name: %w", d.kind.name, err)
		}
		if current == nil {
			return nil, OriginNone, nil
		}
		if n.owned != nil && !n.owned(d.kind.tags(current)) {
			return nil, OriginNone, d.policyOrDefault().foreign(ctx, d.kind.name, n.name, d.kind.id(current), d.route)
		}
		return current, n.origin, nil
	}
}

func (d *discovery[Req, Resp]) lookupTags() locator[Resp] {
	return func(ctx context.Context) (*Resp, Origin, error) {
		tags := []string{BuildTag("env", d.env), BuildTag(parentRoute, d.route)}
		logr.FromContextOrDiscard(ctx).V(1).Info("Looking up kong entity by tags", "entity", d.kind.name, "tags", tags)
		found, err := d.kind.list(ctx, d.api, tags)
		if err != nil {
			return nil, OriginNone, fmt.Errorf("looking up %s by tags: %w", d.kind.name, err)
		}
		switch len(found) {
		case 0:
			return nil, OriginNone, nil
		case 1:
			return &found[0], OriginTags, nil
		default:
			return nil, OriginNone, d.policyOrDefault().ambiguous(ctx, d.kind.name, len(found), tags)
		}
	}
}

// checkConflicts reports to the policy every other entity we own under one of the other
// names, and a foreign entity under the desired name, which would make the rename fail. It runs only when the found entity is not under its desired name.
func (d *discovery[Req, Resp]) checkConflicts(ctx context.Context, found *Resp, named []namedOwnership) error {
	foundID, foundName := d.kind.id(found), d.kind.entName(found)
	for _, n := range named {
		if n.name == foundName {
			continue
		}
		other, err := d.getByName(ctx, n.name)
		if err != nil {
			return fmt.Errorf("checking %s name conflicts: %w", d.kind.name, err)
		}
		if other == nil || d.kind.id(other) == foundID {
			continue
		}
		if n.owned != nil && !n.owned(d.kind.tags(other)) {
			// A foreign entity under the desired name makes the rename impossible.
			if n.name != d.names.Desired {
				continue
			}
			if err := d.policyOrDefault().taken(ctx, d.kind.name, n.name, d.kind.id(other), d.kind.tags(other), foundName, d.route); err != nil {
				return err
			}
			continue
		}
		if err := d.policyOrDefault().duplicate(ctx, d.kind.name, foundName, foundID, n.name, d.kind.id(other), d.route); err != nil {
			return err
		}
	}
	return nil
}
