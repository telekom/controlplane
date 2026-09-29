// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/agenticexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/agenticsubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for AgenticExposure entities in the EdgeCache.
const entityType = "agenticexposure"

// agenticVariantAgent is the AgenticExposure variant that targets AgentCard
// instead of McpServer. All other variants (MCP, TELECONTEXTMCP) target
// McpServer.
const agenticVariantAgent = "AGENT"

// Repository performs typed persistence operations for AgenticExposure entities.
// It implements runtime.Repository[AgenticExposureKey, *AgenticExposureData].
//
// AgenticExposure has a required FK dependency on Application. If the owner
// Application is missing, Upsert returns ErrDependencyMissing.
// Delete removes the entity by base path + application name + team name.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   AgenticExposureDeps
}

// compile-time interface check.
var _ runtime.Repository[AgenticExposureKey, *AgenticExposureData] = (*Repository)(nil)

// NewRepository creates an AgenticExposure repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps AgenticExposureDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an AgenticExposure entity in the database.
// Resolves the owner Application FK (required) via deps, then upserts on
// the composite unique constraint (base_path, owner).
//
// The optional McpServer/AgentCard FK is resolved by Variant: AGENT targets
// AgentCard, all other variants (MCP, TELECONTEXTMCP) target McpServer. Only
// the active catalogue entry for the base path is linked, and only when this
// exposure itself is active — mirroring ApiExposure's Api FK resolution.
func (r *Repository) Upsert(ctx context.Context, data *AgenticExposureData) error {
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

	// Resolve the optional McpServer/AgentCard catalogue FK, mutually
	// exclusive by variant. Only link to the active catalogue entry when this
	// exposure itself is active — mirroring ApiExposure's Api FK resolution.
	var mcpServerID, agentCardID *int
	if data.Active {
		if data.Variant == agenticVariantAgent {
			if resolvedID, findErr := r.deps.FindActiveAgentCardID(ctx, data.BasePath); findErr == nil {
				agentCardID = &resolvedID
			} else if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
				return fmt.Errorf("find active agent_card %q: %w", data.BasePath, findErr)
			}
		} else {
			if resolvedID, findErr := r.deps.FindActiveMcpServerID(ctx, data.BasePath); findErr == nil {
				mcpServerID = &resolvedID
			} else if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
				return fmt.Errorf("find active mcp_server %q: %w", data.BasePath, findErr)
			}
		}
	}

	create := r.client.AgenticExposure.Create().
		SetBasePath(data.BasePath).
		SetVisibility(agenticexposure.Visibility(data.Visibility)).
		SetVariant(agenticexposure.Variant(data.Variant)).
		SetActive(data.Active).
		SetStatusPhase(agenticexposure.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetOwnerID(appID).
		SetUpstreams(data.Upstreams).
		SetApprovalConfig(data.ApprovalConfig)

	if data.Security != nil {
		create.SetSecurity(*data.Security)
	} else {
		create.SetSecurity(model.AgenticExposureSecurity{})
	}

	if data.Traffic != nil {
		create.SetTraffic(*data.Traffic)
	} else {
		create.SetTraffic(model.Traffic{})
	}

	if data.Transformation != nil {
		create.SetTransformation(*data.Transformation)
	} else {
		create.SetTransformation(model.AgenticTransformation{})
	}

	if mcpServerID != nil {
		create.SetNillableMcpServerID(mcpServerID)
	}
	if agentCardID != nil {
		create.SetNillableAgentCardID(agentCardID)
	}

	exposureID, upsertErr := create.
		OnConflictColumns(agenticexposure.FieldBasePath, agenticexposure.OwnerColumn).
		UpdateNewValues().
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert agentic_exposure %q (app %q, team %q): %w",
			data.BasePath, data.AppName, data.TeamName, upsertErr)
	}

	// Explicitly update the catalogue FKs. Like the edge-based subscription FK
	// in the Approval repository, McpServer/AgentCard are edges, not fields, so
	// they are not included in UpdateNewValues() ON CONFLICT SET clauses. On
	// the initial INSERT the FK is set correctly via the edge spec, but on
	// subsequent upserts the old value would be preserved without this —
	// leaving a stale/mutually-inconsistent FK across variant changes, active
	// state transitions, or catalogue entry replacement.
	update := r.client.AgenticExposure.UpdateOneID(exposureID)
	switch {
	case mcpServerID != nil:
		update = update.SetMcpServerID(*mcpServerID).ClearAgentCard()
	case agentCardID != nil:
		update = update.SetAgentCardID(*agentCardID).ClearMcpServer()
	default:
		update = update.ClearMcpServer().ClearAgentCard()
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("update catalogue FK for agentic_exposure %d (%q, app %q, team %q): %w",
			exposureID, data.BasePath, data.AppName, data.TeamName, err)
	}

	et, lk := cachekeys.AgenticExposure(data.BasePath, data.AppName, data.TeamName)
	r.cache.Set(et, lk, exposureID)

	// Back-link subscriptions that were projected before this exposure existed.
	// A subscription targets an active exposure by base path (see
	// FindAgenticExposureByBasePath). If the subscription was reconciled first,
	// it was stored with a NULL target FK and nothing re-links it when the
	// exposure later appears — the subscription CR is not re-reconciled. Here
	// we adopt any such orphaned subscriptions so their target resolves.
	if data.Active {
		if err := r.client.AgenticSubscription.Update().
			Where(
				agenticsubscription.BasePathEQ(data.BasePath),
				agenticsubscription.Not(agenticsubscription.HasTarget()),
			).
			SetTargetID(exposureID).
			Exec(ctx); err != nil {
			return fmt.Errorf("back-link subscriptions to agentic_exposure %q (app %q, team %q): %w",
				data.BasePath, data.AppName, data.TeamName, err)
		}
	}
	return nil
}

// Delete removes an AgenticExposure entity from the database by base path,
// application name, and team name. Returns nil if the entity does not exist
// (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key AgenticExposureKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.AgenticExposure.Delete().
		Where(
			agenticexposure.BasePathEQ(key.BasePath),
			agenticexposure.HasOwnerWith(
				application.NameEQ(key.AppName),
				application.HasOwnerTeamWith(team.NameEQ(key.TeamName)),
			),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete agentic_exposure %q (app %q, team %q): %w",
			key.BasePath, key.AppName, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.AgenticExposure(key.BasePath, key.AppName, key.TeamName)
		r.cache.Del(et, lk)
	}
	return nil
}
