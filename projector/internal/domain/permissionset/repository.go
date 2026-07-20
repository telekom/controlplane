// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package permissionset

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/permissionset"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for PermissionSet entities in the EdgeCache.
const entityType = "permissionset"

// Repository performs typed persistence operations for PermissionSet entities.
// It implements runtime.Repository[PermissionSetKey, *PermissionSetData].
//
// PermissionSet has a required FK dependency on Application. If the owner
// Application is missing, Upsert returns ErrDependencyMissing.
// Delete removes the entity by owning application name + team name.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   PermissionSetDeps
}

// compile-time interface check.
var _ runtime.Repository[PermissionSetKey, *PermissionSetData] = (*Repository)(nil)

// NewRepository creates a PermissionSet repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps PermissionSetDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates a PermissionSet entity in the database.
// Resolves the owner Application FK (required) via deps, then upserts on
// the owner_application unique constraint.
//
// The conflict resolution uses UpdateNewValues() which generates
// "column = EXCLUDED.column" SQL for all insert columns.
func (r *Repository) Upsert(ctx context.Context, data *PermissionSetData) error {
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

	create := r.client.PermissionSet.Create().
		SetPermissions(data.Permissions).
		SetStatusPhase(permissionset.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetOwnerApplicationID(appID)

	permissionSetID, upsertErr := create.
		OnConflictColumns(permissionset.OwnerApplicationColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert permission_set (app %q, team %q): %w",
			data.AppName, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.PermissionSet(data.AppName, data.TeamName)
	r.cache.Set(et, lk, permissionSetID)
	return nil
}

// Delete removes a PermissionSet entity from the database by owning
// application name and team name. Returns nil if the entity does not exist
// (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key PermissionSetKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.PermissionSet.Delete().
		Where(
			permissionset.HasOwnerApplicationWith(
				application.NameEQ(key.AppName),
				application.HasOwnerTeamWith(team.NameEQ(key.TeamName)),
			),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete permission_set (app %q, team %q): %w",
			key.AppName, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.PermissionSet(key.AppName, key.TeamName)
		r.cache.Del(et, lk)
	}
	return nil
}
