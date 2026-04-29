// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for Zone entities in the EdgeCache.
const entityType = "zone"

// Repository performs typed persistence operations for Zone entities.
// It implements runtime.Repository[ZoneKey, *ZoneData].
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
}

// compile-time interface check.
var _ runtime.Repository[ZoneKey, *ZoneData] = (*Repository)(nil)

// NewRepository creates a Zone repository wired with the given ent client and edge cache.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
	}
}

// Upsert creates or updates a Zone entity in the database.
// The upsert conflict target is the unique "name" field.
// GatewayURL is nillable — a nil value clears the field.
func (r *Repository) Upsert(ctx context.Context, data *ZoneData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	create := r.client.Zone.Create().
		SetName(data.Name).
		SetVisibility(zone.Visibility(data.Visibility)).
		SetEnvironment(data.Meta.Environment)

	if data.GatewayURL != nil {
		create = create.SetGatewayURL(*data.GatewayURL)
	}

	id, err := create.
		OnConflictColumns(zone.FieldName).
		Update(func(u *ent.ZoneUpsert) {
			u.SetVisibility(zone.Visibility(data.Visibility))
			u.SetEnvironment(data.Meta.Environment)
			if data.GatewayURL != nil {
				u.SetGatewayURL(*data.GatewayURL)
			} else {
				u.ClearGatewayURL()
			}
		}).
		ID(ctx)
	if err != nil {
		return fmt.Errorf("upsert zone %q: %w", data.Name, err)
	}

	et, lk := cachekeys.Zone(data.Name)
	r.cache.Set(et, lk, id)
	return nil
}

// Delete removes a Zone entity from the database by name.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key ZoneKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	deleted, err := r.client.Zone.Delete().
		Where(zone.NameEQ(string(key))).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete zone %q: %w", key, err)
	}
	if deleted > 0 {
		et, lk := cachekeys.Zone(string(key))
		r.cache.Del(et, lk)
	}
	return nil
}
