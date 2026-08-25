// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entmcpserver "github.com/telekom/controlplane/controlplane-api/ent/mcpserver"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for McpServer entities in the EdgeCache.
const entityType = "mcpserver"

// Repository performs typed persistence operations for McpServer catalogue
// entities. It implements runtime.Repository[McpServerKey, *McpServerData].
//
// McpServer has a required FK dependency on Team. If the owner Team is
// missing, Upsert returns ErrDependencyMissing.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   McpServerDeps
}

// compile-time interface check.
var _ runtime.Repository[McpServerKey, *McpServerData] = (*Repository)(nil)

// NewRepository creates an McpServer repository wired with the given ent
// client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps McpServerDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an McpServer catalogue entity in the database.
// Resolves the owner Team FK (required) via deps, then upserts on the
// composite unique constraint (base_path, owner).
func (r *Repository) Upsert(ctx context.Context, data *McpServerData) error {
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

	create := r.client.McpServer.Create().
		SetBasePath(data.BasePath).
		SetVersion(data.Version).
		SetName(data.Name).
		SetActive(data.Active).
		SetStatusPhase(entmcpserver.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetNamespace(data.Meta.Namespace).
		SetOauth2Scopes(data.Oauth2Scopes).
		SetOwnerID(teamID)

	if data.Description != "" {
		create.SetDescription(data.Description)
	}

	if data.Category != "" {
		create.SetCategory(data.Category)
	}

	if data.Specification != "" {
		create.SetSpecification(data.Specification)
	}

	mcpServerID, upsertErr := create.
		OnConflictColumns(entmcpserver.FieldBasePath, entmcpserver.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert mcp_server %q (team %q): %w",
			data.BasePath, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.McpServer(data.BasePath, data.TeamName)
	r.cache.Set(et, lk, mcpServerID)

	// Update the active-mcp-server cache entry so that AgenticExposure FK
	// resolution can find the active McpServer by base path alone.
	if data.Active {
		aet, alk := cachekeys.ActiveMcpServer(data.BasePath)
		r.cache.Set(aet, alk, mcpServerID)
	} else {
		// If this McpServer is not active, clear the active cache in case it
		// was previously active (should not happen in practice due to
		// oldest-wins, but handles edge cases during resync).
		aet, alk := cachekeys.ActiveMcpServer(data.BasePath)
		r.cache.Del(aet, alk)
	}
	return nil
}

// Delete removes an McpServer catalogue entity from the database by base
// path and team name. Returns nil if the entity does not exist (idempotent
// delete).
func (r *Repository) Delete(ctx context.Context, key McpServerKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.McpServer.Delete().
		Where(
			entmcpserver.BasePathEQ(key.BasePath),
			entmcpserver.HasOwnerWith(team.NameEQ(key.TeamName)),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete mcp_server %q (team %q): %w",
			key.BasePath, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.McpServer(key.BasePath, key.TeamName)
		r.cache.Del(et, lk)
		// Also clear the active-mcp-server cache — if this was the active
		// McpServer, the cache entry is now stale.
		aet, alk := cachekeys.ActiveMcpServer(key.BasePath)
		r.cache.Del(aet, alk)
	}
	return nil
}
