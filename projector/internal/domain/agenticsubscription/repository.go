// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/agenticsubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for AgenticSubscription entities in the EdgeCache.
const entityType = "agenticsubscription"

// fkTargetExposure is the DB FK constraint linking an AgenticSubscription to
// its target AgenticExposure. Matched by name so an FK violation on the
// (required) owner FK is not misread as a stale target. See ent migrate schema.
const fkTargetExposure = "agentic_subscriptions_agentic_exposures_target"

// Repository performs typed persistence operations for AgenticSubscription
// entities. It implements
// runtime.Repository[AgenticSubscriptionKey, *AgenticSubscriptionData].
//
// AgenticSubscription has a required FK dependency on Application (owner) and
// an optional FK dependency on AgenticExposure (target). Special behaviors:
//  1. Target AgenticExposure FK is optional — missing target results in nil
//     FK, not error.
//  2. After upsert, explicitly clears the target FK when the target is nil,
//     because ent's SetNillableTargetID(nil) omits the column from INSERT and
//     UpdateNewValues() cannot clear it on conflict.
//  3. Single cache entry keyed by k8s metadata (namespace, name) for
//     Approval/ApprovalRequest spec.target references.
//  4. Delete cleans the cache entry.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   AgenticSubscriptionDeps
}

// compile-time interface check.
var _ runtime.Repository[AgenticSubscriptionKey, *AgenticSubscriptionData] = (*Repository)(nil)

// NewRepository creates an AgenticSubscription repository wired with the
// given ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps AgenticSubscriptionDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an AgenticSubscription entity in the database.
//
// Steps:
//  1. Resolve owner Application FK (required) — ErrDependencyMissing if missing.
//  2. Resolve target AgenticExposure FK (optional) — nil FK if missing, only
//     non-ErrEntityNotFound errors are propagated.
//  3. Create with ON CONFLICT (base_path, owner) + UpdateNewValues().
//  4. If target is nil, explicitly clear the target FK via ClearTarget().
//     This is necessary because ent's SetNillableTargetID(nil) omits the
//     target column from the INSERT, so UpdateNewValues() cannot clear it
//     on conflict — the old value would be silently preserved.
//  5. Write meta cache entry (namespace, name).
func (r *Repository) Upsert(ctx context.Context, data *AgenticSubscriptionData) error {
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
	// MCP server/agent is exposed. If not found, store with NULL target FK.
	var targetExposureID *int
	if id, findErr := r.deps.FindAgenticExposureByBasePath(ctx, data.TargetBasePath); findErr != nil {
		if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find target agentic_exposure for subscription (basePath %q): %w",
				data.TargetBasePath, findErr)
		}
		// Not found — leave targetExposureID as nil.
	} else {
		targetExposureID = &id
	}

	create := r.client.AgenticSubscription.Create().
		SetBasePath(data.BasePath).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetName(data.Meta.Name).
		SetStatusPhase(agenticsubscription.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetOwnerID(ownerAppID).
		SetNillableTargetID(targetExposureID)

	if data.Security != nil {
		create.SetSecurity(*data.Security)
	} else {
		create.SetSecurity(model.AgenticSubscriptionSecurity{})
	}

	if data.Traffic != nil {
		create.SetTraffic(*data.Traffic)
	} else {
		create.SetTraffic(model.AgenticSubscriberTraffic{})
	}

	subscriptionID, upsertErr := create.
		OnConflictColumns(agenticsubscription.FieldBasePath, agenticsubscription.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		// A FK violation on the target means the cached exposure ID is stale:
		// the agentic_exposures row was deleted/re-created after we resolved
		// it. Evict the stale cache entry and requeue as dependency-missing so
		// the next reconcile re-resolves (or stores a NULL target).
		if targetExposureID != nil && infrastructure.IsFKViolation(upsertErr, fkTargetExposure) {
			r.deps.EvictAgenticExposureByBasePath(data.TargetBasePath)
			return runtime.WrapDependencyMissing("agentic_exposure", data.TargetBasePath)
		}
		return fmt.Errorf("upsert agentic_subscription (owner %q, basePath %q): %w",
			data.OwnerAppName, data.BasePath, upsertErr)
	}

	// When targetExposureID is nil, ent's SetNillableTargetID(nil) omits the
	// target column from the INSERT statement entirely. On conflict,
	// UpdateNewValues() only generates SET clauses for columns present in the
	// INSERT, so the old target FK value would be preserved instead of being
	// cleared to NULL. We explicitly clear it here.
	if targetExposureID == nil {
		if err := r.client.AgenticSubscription.UpdateOneID(subscriptionID).
			ClearTarget().
			Exec(ctx); err != nil {
			return fmt.Errorf("clear target FK for agentic_subscription %d (owner %q, basePath %q): %w",
				subscriptionID, data.OwnerAppName, data.BasePath, err)
		}
	}

	// Cache entry keyed by k8s metadata for Approval/ApprovalRequest
	// spec.target references.
	et, lk := cachekeys.AgenticSubscriptionMeta(data.Meta.Namespace, data.Meta.Name)
	r.cache.Set(et, lk, subscriptionID)
	return nil
}

// Delete removes an AgenticSubscription entity from the database by owner
// application name, team name, and base path. Also cleans the meta cache
// entry if the entity was found and namespace/name are available.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key AgenticSubscriptionKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.AgenticSubscription.Delete().
		Where(
			agenticsubscription.HasOwnerWith(
				application.NameEQ(key.OwnerAppName),
				application.HasOwnerTeamWith(team.NameEQ(key.OwnerTeamName)),
			),
			agenticsubscription.BasePathEQ(key.BasePath),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete agentic_subscription (owner %q, team %q, basePath %q): %w",
			key.OwnerAppName, key.OwnerTeamName, key.BasePath, err)
	}
	if count > 0 {
		if key.Namespace != "" && key.Name != "" {
			et, lk := cachekeys.AgenticSubscriptionMeta(key.Namespace, key.Name)
			r.cache.Del(et, lk)
		}
	}
	return nil
}
