// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agentcard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entagentcard "github.com/telekom/controlplane/controlplane-api/ent/agentcard"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for AgentCard entities in the EdgeCache.
const entityType = "agentcard"

// Repository performs typed persistence operations for AgentCard catalogue
// entities. It implements runtime.Repository[AgentCardKey, *AgentCardData].
//
// AgentCard has a required FK dependency on Team. If the owner Team is
// missing, Upsert returns ErrDependencyMissing.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   AgentCardDeps
}

// compile-time interface check.
var _ runtime.Repository[AgentCardKey, *AgentCardData] = (*Repository)(nil)

// NewRepository creates an AgentCard repository wired with the given ent
// client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps AgentCardDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an AgentCard catalogue entity in the database.
// Resolves the owner Team FK (required) via deps, then upserts on the
// composite unique constraint (base_path, owner).
func (r *Repository) Upsert(ctx context.Context, data *AgentCardData) error {
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

	create := r.client.AgentCard.Create().
		SetBasePath(data.BasePath).
		SetVersion(data.Version).
		SetName(data.Name).
		SetActive(data.Active).
		SetStatusPhase(entagentcard.StatusPhase(data.StatusPhase)).
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

	agentCardID, upsertErr := create.
		OnConflictColumns(entagentcard.FieldBasePath, entagentcard.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert agent_card %q (team %q): %w",
			data.BasePath, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.AgentCard(data.BasePath, data.TeamName)
	r.cache.Set(et, lk, agentCardID)

	// Update the active-agent-card cache entry so that AgenticExposure FK
	// resolution can find the active AgentCard by base path alone.
	if data.Active {
		aet, alk := cachekeys.ActiveAgentCard(data.BasePath)
		r.cache.Set(aet, alk, agentCardID)
	} else {
		// If this AgentCard is not active, clear the active cache in case it
		// was previously active (should not happen in practice due to
		// oldest-wins, but handles edge cases during resync).
		aet, alk := cachekeys.ActiveAgentCard(data.BasePath)
		r.cache.Del(aet, alk)
	}
	return nil
}

// Delete removes an AgentCard catalogue entity from the database by base
// path and team name. Returns nil if the entity does not exist (idempotent
// delete).
func (r *Repository) Delete(ctx context.Context, key AgentCardKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.AgentCard.Delete().
		Where(
			entagentcard.BasePathEQ(key.BasePath),
			entagentcard.HasOwnerWith(team.NameEQ(key.TeamName)),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete agent_card %q (team %q): %w",
			key.BasePath, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.AgentCard(key.BasePath, key.TeamName)
		r.cache.Del(et, lk)
		// Also clear the active-agent-card cache — if this was the active
		// AgentCard, the cache entry is now stale.
		aet, alk := cachekeys.ActiveAgentCard(key.BasePath)
		r.cache.Del(aet, alk)
	}
	return nil
}
