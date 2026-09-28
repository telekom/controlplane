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
	"github.com/telekom/controlplane/controlplane-api/ent/agenticexposure"
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
// entities. It implements runtime.Repository[MCPServerKey, *MCPServerData].
//
// McpServer has a required FK dependency on Team. If the owner Team is
// missing, Upsert returns ErrDependencyMissing.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   MCPServerDeps
}

// compile-time interface check.
var _ runtime.Repository[MCPServerKey, *MCPServerData] = (*Repository)(nil)

// NewRepository creates an McpServer repository wired with the given ent
// client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps MCPServerDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an McpServer catalogue entity in the database.
// Resolves the owner Team FK (required) via deps, then upserts on the
// composite unique constraint (base_path, owner).
func (r *Repository) Upsert(ctx context.Context, data *MCPServerData) error {
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

	create := r.client.MCPServer.Create().
		SetBasePath(data.BasePath).
		SetVersion(data.Version).
		SetName(data.Name).
		SetActive(data.Active).
		SetStatusPhase(entmcpserver.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetNamespace(data.Meta.Namespace).
		SetOAuth2Scopes(data.OAuth2Scopes).
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

	et, lk := cachekeys.MCPServer(data.BasePath, data.TeamName)
	r.cache.Set(et, lk, mcpServerID)

	// Update the active-mcp-server cache entry so that AgenticExposure FK
	// resolution can find the active McpServer by base path alone, and
	// back-link any AgenticExposures that were projected before this McpServer
	// existed. An MCP/TELECONTEXTMCP-variant AgenticExposure targets the active
	// McpServer by base path (see FindActiveMCPServerID). If the exposure was
	// reconciled first, it was stored with a NULL mcp_server FK and nothing
	// re-links it when the McpServer later becomes active — the exposure CR is
	// not re-reconciled.
	if data.Active {
		aet, alk := cachekeys.ActiveMCPServer(data.BasePath)
		r.cache.Set(aet, alk, mcpServerID)

		if _, err := r.client.AgenticExposure.Update().
			Where(
				agenticexposure.BasePathEQ(data.BasePath),
				agenticexposure.VariantIn(agenticexposure.VariantMCP, agenticexposure.VariantTelecontextMCP),
				agenticexposure.ActiveEQ(true),
				agenticexposure.Not(agenticexposure.HasMCPServer()),
			).
			SetMCPServerID(mcpServerID).
			Save(ctx); err != nil {
			return fmt.Errorf("back-link agentic_exposures to mcp_server %q: %w", data.BasePath, err)
		}
	} else {
		// Only clear the active cache entry if it currently points to this McpServer.
		aet, alk := cachekeys.ActiveMCPServer(data.BasePath)
		if cachedID, ok := r.cache.Get(aet, alk); ok && cachedID == mcpServerID {
			r.cache.Del(aet, alk)
		}
	}
	return nil
}

// Delete removes an McpServer catalogue entity from the database by base
// path and team name. Returns nil if the entity does not exist (idempotent
// delete).
func (r *Repository) Delete(ctx context.Context, key MCPServerKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.MCPServer.Delete().
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
		et, lk := cachekeys.MCPServer(key.BasePath, key.TeamName)
		r.cache.Del(et, lk)
		// Also clear the active-mcp-server cache — if this was the active
		// McpServer, the cache entry is now stale.
		aet, alk := cachekeys.ActiveMCPServer(key.BasePath)
		r.cache.Del(aet, alk)
	}
	return nil
}
