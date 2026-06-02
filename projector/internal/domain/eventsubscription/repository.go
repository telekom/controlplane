// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/eventsubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for EventSubscription entities in the EdgeCache.
const entityType = "eventsubscription"

// Repository performs typed persistence operations for EventSubscription entities.
// It implements runtime.Repository[EventSubscriptionKey, *EventSubscriptionData].
//
// EventSubscription has a required FK dependency on Application (owner) and an
// optional FK dependency on EventExposure (target).
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   EventSubscriptionDeps
}

// compile-time interface check.
var _ runtime.Repository[EventSubscriptionKey, *EventSubscriptionData] = (*Repository)(nil)

// NewRepository creates an EventSubscription repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps EventSubscriptionDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an EventSubscription entity in the database.
//
// Steps:
//  1. Resolve owner Application FK (required) — ErrDependencyMissing if missing.
//  2. Resolve target EventExposure FK (optional) — nil FK if missing.
//  3. Create with ON CONFLICT (event_type, owner) + UpdateNewValues().
//  4. If target is nil, explicitly clear the target FK via ClearTarget().
//  5. Write meta cache entry (namespace, name).
func (r *Repository) Upsert(ctx context.Context, data *EventSubscriptionData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	ownerAppID, err := r.deps.FindApplicationID(ctx, data.OwnerAppName, data.OwnerTeamName)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("application", data.OwnerAppName)
		}
		return fmt.Errorf("find application %q (team %q): %w", data.OwnerAppName, data.OwnerTeamName, err)
	}

	// Target exposure is optional — subscription may exist before the target
	// event is exposed.
	var targetExposureID *int
	if id, findErr := r.deps.FindEventExposureByEventType(ctx, data.TargetEventType); findErr != nil {
		if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find target event_exposure for subscription (eventType %q): %w",
				data.TargetEventType, findErr)
		}
	} else {
		targetExposureID = &id
	}

	create := r.client.EventSubscription.Create().
		SetEventType(data.EventType).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetName(data.Meta.Name).
		SetDeliveryType(eventsubscription.DeliveryType(data.DeliveryType)).
		SetNillableCallbackURL(data.CallbackURL).
		SetStatusPhase(eventsubscription.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetOwnerID(ownerAppID).
		SetNillableTargetID(targetExposureID)

	subscriptionID, upsertErr := create.
		OnConflictColumns(eventsubscription.FieldEventType, eventsubscription.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert event_subscription (owner %q, eventType %q): %w",
			data.OwnerAppName, data.EventType, upsertErr)
	}

	// When targetExposureID is nil, explicitly clear the target FK.
	if targetExposureID == nil {
		if err := r.client.EventSubscription.UpdateOneID(subscriptionID).
			ClearTarget().
			Exec(ctx); err != nil {
			return fmt.Errorf("clear target FK for event_subscription %d (owner %q, eventType %q): %w",
				subscriptionID, data.OwnerAppName, data.EventType, err)
		}
	}

	et, lk := cachekeys.EventSubscriptionMeta(data.Meta.Namespace, data.Meta.Name)
	r.cache.Set(et, lk, subscriptionID)
	return nil
}

// Delete removes an EventSubscription entity from the database by owner
// application name, team name, and event type. Also cleans the meta cache
// entry. Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key EventSubscriptionKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.EventSubscription.Delete().
		Where(
			eventsubscription.HasOwnerWith(
				application.NameEQ(key.OwnerAppName),
				application.HasOwnerTeamWith(team.NameEQ(key.OwnerTeamName)),
			),
			eventsubscription.EventTypeEQ(key.EventType),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete event_subscription (owner %q, team %q, eventType %q): %w",
			key.OwnerAppName, key.OwnerTeamName, key.EventType, err)
	}
	if count > 0 {
		if key.Namespace != "" && key.Name != "" {
			et, lk := cachekeys.EventSubscriptionMeta(key.Namespace, key.Name)
			r.cache.Del(et, lk)
		}
	}
	return nil
}
