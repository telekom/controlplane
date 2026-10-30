// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"fmt"
	"slices"
)

// locatedEntity decorates a plain entity of a realm route. Get runs the discovery chain,
// Write puts an entity that was found by its ID, so that it is renamed in the same write,
// and creates a new entity under the desired name. The Java tags of a found entity are
// kept: they are merged into the desired tags for both the comparison and the write.
type locatedEntity[Req, Resp any] struct {
	discovery *discovery[Req, Resp]

	located bool
	current *Resp
}

var _ comparer[any] = &locatedEntity[any, any]{}

func newLocatedEntity[Req, Resp any](d *discovery[Req, Resp]) *locatedEntity[Req, Resp] {
	return &locatedEntity[Req, Resp]{discovery: d}
}

func (e *locatedEntity[Req, Resp]) Name() string { return e.discovery.kind.name }

// locate runs the discovery once and remembers its result.
func (e *locatedEntity[Req, Resp]) locate(ctx context.Context) (*Resp, error) {
	if e.located {
		return e.current, nil
	}
	current, _, err := e.discovery.locate(ctx)
	if err != nil {
		return nil, err
	}
	e.located, e.current = true, current
	return current, nil
}

// foundID returns the Kong ID of the found entity, or "" if none was found.
func (e *locatedEntity[Req, Resp]) foundID() string {
	if e.current == nil {
		return ""
	}
	return e.discovery.kind.id(e.current)
}

// legacyName returns the name the entity was found under if it is a name from before the
// migration: metadata.name or a Java candidate. Steps 1–3 may find such an entity as well.
func (e *locatedEntity[Req, Resp]) legacyName() string {
	if e.current == nil {
		return ""
	}
	name := e.discovery.kind.entName(e.current)
	if name == e.discovery.names.PreMigration {
		return name
	}
	for _, candidate := range e.discovery.names.Candidates {
		if candidate.Name == name {
			return name
		}
	}
	return ""
}

func (e *locatedEntity[Req, Resp]) Get(ctx context.Context) (*Resp, bool, error) {
	current, err := e.locate(ctx)
	if err != nil {
		return nil, false, err
	}
	return current, current != nil, nil
}

func (e *locatedEntity[Req, Resp]) Project(current *Resp) (Req, error) {
	return e.discovery.kind.plain(e.discovery.api, "").Project(current)
}

// Equal compares the desired body with the Java tags of the found entity merged in.
func (e *locatedEntity[Req, Resp]) Equal(desired, current Req) bool {
	desired = e.withLegacyTags(desired)
	return equal(e.discovery.kind.plain(e.discovery.api, ""), desired, current)
}

func (e *locatedEntity[Req, Resp]) Write(ctx context.Context, desired *Req) (*Resp, error) {
	merged := e.withLegacyTags(*desired)
	idOrName := e.discovery.names.Desired
	if id := e.foundID(); id != "" {
		idOrName = id
	}
	written, err := e.discovery.kind.plain(e.discovery.api, idOrName).Write(ctx, &merged)
	if err != nil {
		return nil, fmt.Errorf("writing located %s: %w", e.discovery.kind.name, err)
	}
	return written, nil
}

func (e *locatedEntity[Req, Resp]) withLegacyTags(desired Req) Req {
	if e.current == nil {
		return desired
	}
	kept := legacyTags(e.discovery.kind.tags(e.current))
	if len(kept) == 0 {
		return desired
	}
	desiredTags := e.discovery.kind.reqTags(&desired)
	tags := slices.Concat(valueOrZero(*desiredTags), kept)
	*desiredTags = normalizeSet(&tags)
	return desired
}
