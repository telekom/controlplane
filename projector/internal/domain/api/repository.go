// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entapi "github.com/telekom/controlplane/controlplane-api/ent/api"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for Api entities in the EdgeCache.
const entityType = "api"

// Repository performs typed persistence operations for Api catalogue entities.
// It implements runtime.Repository[ApiKey, *ApiData].
//
// Api has a required FK dependency on Team. If the owner Team is missing,
// Upsert returns ErrDependencyMissing.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   ApiDeps
}

// compile-time interface check.
var _ runtime.Repository[ApiKey, *ApiData] = (*Repository)(nil)

// NewRepository creates an Api repository wired with the given ent client,
// edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps ApiDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an Api catalogue entity in the database.
// Resolves the owner Team FK (required) via deps, then upserts on the
// composite unique constraint (base_path, owner).
func (r *Repository) Upsert(ctx context.Context, data *ApiData) error {
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

	create := r.client.Api.Create().
		SetBasePath(data.BasePath).
		SetVersion(data.Version).
		SetActive(data.Active).
		SetStatusPhase(entapi.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetNamespace(data.Meta.Namespace).
		SetXVendor(data.XVendor).
		SetOauth2Scopes(data.Oauth2Scopes).
		SetOwnerID(teamID)

	if data.Category != "" {
		create.SetCategory(data.Category)
	}

	if data.Specification != "" {
		create.SetSpecification(data.Specification)
	}

	apiID, upsertErr := create.
		OnConflictColumns(entapi.FieldBasePath, entapi.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert api %q (team %q): %w",
			data.BasePath, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.Api(data.BasePath, data.TeamName)
	r.cache.Set(et, lk, apiID)

	// Update the active-api cache entry so that ApiExposure FK resolution
	// can find the active Api by base path alone.
	if data.Active {
		aet, alk := cachekeys.ActiveApi(data.BasePath)
		r.cache.Set(aet, alk, apiID)
	} else {
		// If this Api is not active, clear the active cache in case it was
		// previously active (should not happen in practice due to oldest-wins,
		// but handles edge cases during resync).
		aet, alk := cachekeys.ActiveApi(data.BasePath)
		r.cache.Del(aet, alk)
	}
	return nil
}

// Delete removes an Api catalogue entity from the database by base path and
// team name. Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key ApiKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.Api.Delete().
		Where(
			entapi.BasePathEQ(key.BasePath),
			entapi.HasOwnerWith(team.NameEQ(key.TeamName)),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete api %q (team %q): %w",
			key.BasePath, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.Api(key.BasePath, key.TeamName)
		r.cache.Del(et, lk)
		// Also clear the active-api cache — if this was the active Api,
		// the cache entry is now stale.
		aet, alk := cachekeys.ActiveApi(key.BasePath)
		r.cache.Del(aet, alk)
	}
	return nil
}
