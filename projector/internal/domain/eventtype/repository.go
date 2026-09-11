// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	enteventtype "github.com/telekom/controlplane/controlplane-api/ent/eventtype"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for EventType entities in the EdgeCache.
const entityType = "eventtype"

// Repository performs typed persistence operations for EventType catalogue entities.
// It implements runtime.Repository[EventTypeKey, *EventTypeData].
//
// EventType has a required FK dependency on Team. If the owner Team is missing,
// Upsert returns ErrDependencyMissing.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   EventTypeDeps
}

// compile-time interface check.
var _ runtime.Repository[EventTypeKey, *EventTypeData] = (*Repository)(nil)

// NewRepository creates an EventType repository wired with the given ent client,
// edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps EventTypeDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an EventType catalogue entity in the database.
// Resolves the owner Team FK (required) via deps, then upserts on the
// composite unique constraint (event_type, owner).
func (r *Repository) Upsert(ctx context.Context, data *EventTypeData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	teamID, err := r.deps.FindTeamID(ctx, data.TeamName)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("team", data.TeamName)
		}
		return fmt.Errorf("find team %q: %w", data.TeamName, err)
	}

	create := r.client.EventType.Create().
		SetEventType(data.EventType).
		SetVersion(data.Version).
		SetActive(data.Active).
		SetStatusPhase(enteventtype.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetNamespace(data.Meta.Namespace).
		SetOwnerID(teamID)

	if data.Description != "" {
		create.SetDescription(data.Description)
	}

	if data.Specification != "" {
		create.SetSpecification(data.Specification)
	}

	eventTypeID, upsertErr := create.
		OnConflictColumns(enteventtype.FieldEventType, enteventtype.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert event_type %q (team %q): %w",
			data.EventType, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.EventTypeDef(data.EventType, data.TeamName)
	r.cache.Set(et, lk, eventTypeID)

	// Update the active-eventtype cache entry so that EventExposure FK resolution
	// can find the active EventType by type string alone.
	if data.Active {
		aet, alk := cachekeys.ActiveEventType(data.EventType)
		r.cache.Set(aet, alk, eventTypeID)
	} else {
		aet, alk := cachekeys.ActiveEventType(data.EventType)
		r.cache.Del(aet, alk)
	}
	return nil
}

// Delete removes an EventType catalogue entity from the database by event type
// and team name. Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key EventTypeKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.EventType.Delete().
		Where(
			enteventtype.EventTypeEQ(key.EventType),
			enteventtype.HasOwnerWith(team.NameEQ(key.TeamName)),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete event_type %q (team %q): %w",
			key.EventType, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.EventTypeDef(key.EventType, key.TeamName)
		r.cache.Del(et, lk)
		aet, alk := cachekeys.ActiveEventType(key.EventType)
		r.cache.Del(aet, alk)
	}
	return nil
}
