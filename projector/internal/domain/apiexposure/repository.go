// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for ApiExposure entities in the EdgeCache.
const entityType = "apiexposure"

// Repository performs typed persistence operations for ApiExposure entities.
// It implements runtime.Repository[APIExposureKey, *APIExposureData].
//
// ApiExposure has a required FK dependency on Application. If the owner
// Application is missing, Upsert returns ErrDependencyMissing.
// Delete removes the entity by base path + application name + team name.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   APIExposureDeps
}

// compile-time interface check.
var _ runtime.Repository[APIExposureKey, *APIExposureData] = (*Repository)(nil)

// NewRepository creates an ApiExposure repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps APIExposureDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an ApiExposure entity in the database.
// Resolves the owner Application FK (required) via deps, then upserts on
// the composite unique constraint (base_path, owner).
//
// The conflict resolution uses UpdateNewValues() which generates
// "column = EXCLUDED.column" SQL for all insert columns.
// APIVersion is nillable — set only when non-nil.
func (r *Repository) Upsert(ctx context.Context, data *APIExposureData) error {
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

	create := r.client.ApiExposure.Create().
		SetBasePath(data.BasePath).
		SetVisibility(apiexposure.Visibility(data.Visibility)).
		SetActive(data.Active).
		SetFeatures(data.Features).
		SetStatusPhase(apiexposure.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetOwnerID(appID).
		SetUpstreams(data.Upstreams).
		SetApprovalConfig(data.ApprovalConfig)

	if data.APIVersion != nil {
		create.SetAPIVersion(*data.APIVersion)
	}

	exposureID, upsertErr := create.
		OnConflictColumns(apiexposure.FieldBasePath, apiexposure.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert api_exposure %q (app %q, team %q): %w",
			data.BasePath, data.AppName, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.APIExposure(data.BasePath, data.AppName, data.TeamName)
	r.cache.Set(et, lk, exposureID)
	return nil
}

// Delete removes an ApiExposure entity from the database by base path,
// application name, and team name. Returns nil if the entity does not exist
// (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key APIExposureKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.ApiExposure.Delete().
		Where(
			apiexposure.BasePathEQ(key.BasePath),
			apiexposure.HasOwnerWith(
				application.NameEQ(key.AppName),
				application.HasOwnerTeamWith(team.NameEQ(key.TeamName)),
			),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete api_exposure %q (app %q, team %q): %w",
			key.BasePath, key.AppName, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.APIExposure(key.BasePath, key.AppName, key.TeamName)
		r.cache.Del(et, lk)
	}
	return nil
}
