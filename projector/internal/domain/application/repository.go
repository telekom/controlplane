// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for Application entities in the EdgeCache.
const entityType = "application"

// Repository performs typed persistence operations for Application entities.
// It implements runtime.Repository[ApplicationKey, *ApplicationData].
//
// Application has required FK dependencies on both Team and Zone. If either
// dependency is missing, Upsert returns ErrDependencyMissing. Delete cascades
// to child ApiExposure and ApiSubscription entities.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   ApplicationDeps
}

// compile-time interface check.
var _ runtime.Repository[ApplicationKey, *ApplicationData] = (*Repository)(nil)

// NewRepository creates an Application repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps ApplicationDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates an Application entity in the database.
// Resolves Team FK and Zone FK (both required) via deps, then upserts on
// the composite unique constraint (name, owner_team).
//
// ClientID, ClientSecret and IssuerURL are nillable — set only when non-nil.
// On conflict update: always sets StatusPhase, StatusMessage; conditionally
// sets/clears ClientID, ClientSecret and IssuerURL.
func (r *Repository) Upsert(ctx context.Context, data *ApplicationData) error {
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

	zoneID, err := r.deps.FindZoneID(ctx, data.ZoneName)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("zone", data.ZoneName)
		}
		return fmt.Errorf("find zone %q: %w", data.ZoneName, err)
	}

	create := r.client.Application.Create().
		SetName(data.Name).
		SetStatusPhase(application.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetOwnerTeamID(teamID).
		SetZoneID(zoneID)

	if data.ClientID != nil {
		create.SetClientID(*data.ClientID)
	}
	if data.ClientSecret != nil {
		create.SetClientSecret(*data.ClientSecret)
	}
	if data.IssuerURL != nil {
		create.SetIssuerURL(*data.IssuerURL)
	}

	appID, upsertErr := create.
		OnConflictColumns(application.FieldName, application.OwnerTeamColumn).
		Update(func(u *ent.ApplicationUpsert) {
			u.SetStatusPhase(application.StatusPhase(data.StatusPhase))
			u.SetStatusMessage(data.StatusMessage)
			u.SetEnvironment(data.Meta.Environment)
			u.SetNamespace(data.Meta.Namespace)
			if data.ClientID != nil {
				u.SetClientID(*data.ClientID)
			}
			if data.ClientSecret != nil {
				u.SetClientSecret(*data.ClientSecret)
			}
			if data.IssuerURL != nil {
				u.SetIssuerURL(*data.IssuerURL)
			} else {
				u.ClearIssuerURL()
			}
		}).
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert application %q (team %q): %w", data.Name, data.TeamName, upsertErr)
	}

	et, lk := cachekeys.Application(data.Name, data.TeamName)
	r.cache.Set(et, lk, appID)
	return nil
}

// Delete removes an Application entity from the database by name and team.
// Explicitly deletes associated ApiExposures and ApiSubscriptions before
// removing the Application row, since ent does not configure ON DELETE CASCADE.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key ApplicationKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	app, err := r.client.Application.Query().
		Where(
			application.NameEQ(key.Name),
			application.HasOwnerTeamWith(team.NameEQ(key.TeamName)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("find application %q (team %q) for delete: %w", key.Name, key.TeamName, err)
	}

	// Delete child entities first.
	if _, err := r.client.ApiExposure.Delete().
		Where(apiexposure.HasOwnerWith(application.IDEQ(app.ID))).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete api_exposures for application %q: %w", key.Name, err)
	}
	if _, err := r.client.ApiSubscription.Delete().
		Where(apisubscription.HasOwnerWith(application.IDEQ(app.ID))).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete api_subscriptions for application %q: %w", key.Name, err)
	}

	// Delete the application itself.
	if err := r.client.Application.DeleteOneID(app.ID).Exec(ctx); err != nil {
		return fmt.Errorf("delete application %q: %w", key.Name, err)
	}

	et, lk := cachekeys.Application(key.Name, key.TeamName)
	r.cache.Del(et, lk)
	return nil
}
