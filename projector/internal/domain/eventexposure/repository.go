// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/eventexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for EventExposure entities in the EdgeCache.
const entityType = "eventexposure"

// Repository performs typed persistence operations for EventExposure entities.
// It implements runtime.Repository[EventExposureKey, *EventExposureData].
//
// EventExposure has a required FK dependency on Application. If the owner
// Application is missing, Upsert returns ErrDependencyMissing.
// Delete removes the entity by event type + application name + team name.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   EventExposureDeps
}

// compile-time interface check.
var _ runtime.Repository[EventExposureKey, *EventExposureData] = (*Repository)(nil)

// NewRepository creates an EventExposure repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps EventExposureDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an EventExposure entity in the database.
// Resolves the owner Application FK (required) via deps, then upserts on
// the composite unique constraint (event_type, owner).
func (r *Repository) Upsert(ctx context.Context, data *EventExposureData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	appID, err := r.deps.FindApplicationID(ctx, data.AppName, data.TeamName)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("application", data.AppName)
		}
		return fmt.Errorf("find application %q (team %q): %w", data.AppName, data.TeamName, err)
	}

	// Resolve optional EventType catalogue FK. Only link to the active EventType
	// when this exposure itself is active. The lookup is team-independent because
	// only one EventType is active cluster-wide for a given type string (oldest-wins).
	var eventTypeDefID *int
	if data.Active {
		if resolvedID, etErr := r.deps.FindActiveEventTypeID(ctx, data.EventType); etErr == nil {
			eventTypeDefID = &resolvedID
		} else if !errors.Is(etErr, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find active event_type %q: %w", data.EventType, etErr)
		}
	}

	exposureID, upsertErr := r.client.EventExposure.Create().
		SetEventType(data.EventType).
		SetVisibility(eventexposure.Visibility(data.Visibility)).
		SetActive(data.Active).
		SetStatusPhase(eventexposure.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetOwnerID(appID).
		SetApprovalConfig(data.ApprovalConfig).
		SetNillableEventTypeDefID(eventTypeDefID).
		SetGatewayProviderURL(data.GatewayProviderUrl).
		OnConflictColumns(eventexposure.FieldEventType, eventexposure.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert event_exposure %q (app %q, team %q): %w",
			data.EventType, data.AppName, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.EventExposure(data.EventType, data.AppName, data.TeamName)
	r.cache.Set(et, lk, exposureID)
	return nil
}

// Delete removes an EventExposure entity from the database by event type,
// application name, and team name. Returns nil if the entity does not exist
// (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key EventExposureKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.EventExposure.Delete().
		Where(
			eventexposure.EventTypeEQ(key.EventType),
			eventexposure.HasOwnerWith(
				application.NameEQ(key.AppName),
				application.HasOwnerTeamWith(team.NameEQ(key.TeamName)),
			),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete event_exposure %q (app %q, team %q): %w",
			key.EventType, key.AppName, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.EventExposure(key.EventType, key.AppName, key.TeamName)
		r.cache.Del(et, lk)
	}
	return nil
}
