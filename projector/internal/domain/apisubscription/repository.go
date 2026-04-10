// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for ApiSubscription entities in the EdgeCache.
const entityType = "apisubscription"

// Repository performs typed persistence operations for ApiSubscription entities.
// It implements runtime.Repository[APISubscriptionKey, *APISubscriptionData].
//
// ApiSubscription has a required FK dependency on Application (owner) and an
// optional FK dependency on ApiExposure (target). Special behaviors:
//  1. Target ApiExposure FK is optional — missing target results in nil FK, not error.
//  2. After upsert, explicitly clears the target FK when the target is nil,
//     because ent's SetNillableTargetID(nil) omits the column from INSERT and
//     UpdateNewValues() cannot clear it on conflict.
//  3. Single cache entry keyed by k8s metadata (namespace, name) for
//     Approval/ApprovalRequest spec.target references.
//  4. Delete cleans the cache entry.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   APISubscriptionDeps
}

// compile-time interface check.
var _ runtime.Repository[APISubscriptionKey, *APISubscriptionData] = (*Repository)(nil)

// NewRepository creates an ApiSubscription repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps APISubscriptionDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an ApiSubscription entity in the database.
//
// Steps:
//  1. Resolve owner Application FK (required) — ErrDependencyMissing if missing.
//  2. Resolve target ApiExposure FK (optional) — nil FK if missing, only
//     non-ErrEntityNotFound errors are propagated.
//  3. Create with ON CONFLICT (base_path, owner) + UpdateNewValues().
//  4. If target is nil, explicitly clear the target FK via ClearTarget().
//     This is necessary because ent's SetNillableTargetID(nil) omits the
//     target column from the INSERT, so UpdateNewValues() cannot clear it
//     on conflict — the old value would be silently preserved.
//  5. Write meta cache entry (namespace, name).
func (r *Repository) Upsert(ctx context.Context, data *APISubscriptionData) error {
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
	// API is exposed. If not found, store with NULL target FK.
	var targetExposureID *int
	if id, findErr := r.deps.FindAPIExposureByBasePath(ctx, data.TargetBasePath); findErr != nil {
		if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find target api_exposure for subscription (basePath %q): %w",
				data.TargetBasePath, findErr)
		}
		// Not found — leave targetExposureID as nil.
	} else {
		targetExposureID = &id
	}

	create := r.client.ApiSubscription.Create().
		SetBasePath(data.BasePath).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetName(data.Meta.Name).
		SetM2mAuthMethod(apisubscription.M2mAuthMethod(data.M2MAuthMethod)).
		SetApprovedScopes(data.ApprovedScopes).
		SetStatusPhase(apisubscription.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetOwnerID(ownerAppID).
		SetNillableTargetID(targetExposureID)

	subscriptionID, upsertErr := create.
		OnConflictColumns(apisubscription.FieldBasePath, apisubscription.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert api_subscription (owner %q, basePath %q): %w",
			data.OwnerAppName, data.BasePath, upsertErr)
	}

	// When targetExposureID is nil, ent's SetNillableTargetID(nil) omits the
	// target column from the INSERT statement entirely. On conflict,
	// UpdateNewValues() only generates SET clauses for columns present in the
	// INSERT, so the old target FK value would be preserved instead of being
	// cleared to NULL. We explicitly clear it here.
	if targetExposureID == nil {
		if err := r.client.ApiSubscription.UpdateOneID(subscriptionID).
			ClearTarget().
			Exec(ctx); err != nil {
			return fmt.Errorf("clear target FK for api_subscription %d (owner %q, basePath %q): %w",
				subscriptionID, data.OwnerAppName, data.BasePath, err)
		}
	}

	// Cache entry keyed by k8s metadata for Approval/ApprovalRequest
	// spec.target references.
	et, lk := cachekeys.APISubscriptionMeta(data.Meta.Namespace, data.Meta.Name)
	r.cache.Set(et, lk, subscriptionID)
	return nil
}

// Delete removes an ApiSubscription entity from the database by owner
// application name, team name, and base path. Also cleans the meta cache
// entry if the entity was found and namespace/name are available.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key APISubscriptionKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.ApiSubscription.Delete().
		Where(
			apisubscription.HasOwnerWith(
				application.NameEQ(key.OwnerAppName),
				application.HasOwnerTeamWith(team.NameEQ(key.OwnerTeamName)),
			),
			apisubscription.BasePathEQ(key.BasePath),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete api_subscription (owner %q, team %q, basePath %q): %w",
			key.OwnerAppName, key.OwnerTeamName, key.BasePath, err)
	}
	if count > 0 {
		if key.Namespace != "" && key.Name != "" {
			et, lk := cachekeys.APISubscriptionMeta(key.Namespace, key.Name)
			r.cache.Del(et, lk)
		}
	}
	return nil
}
