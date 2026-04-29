// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entgroup "github.com/telekom/controlplane/controlplane-api/ent/group"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for Group entities in the EdgeCache.
const entityType = "group"

// Repository performs typed persistence operations for Group entities.
// It implements runtime.Repository[GroupKey, *GroupData].
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
}

// compile-time interface check.
var _ runtime.Repository[GroupKey, *GroupData] = (*Repository)(nil)

// NewRepository creates a Group repository wired with the given ent client and edge cache.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
	}
}

// Upsert creates or updates a Group entity in the database.
// The upsert conflict target is the unique "name" field.
func (r *Repository) Upsert(ctx context.Context, data *GroupData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	id, err := r.client.Group.Create().
		SetName(data.Name).
		SetDisplayName(data.DisplayName).
		SetDescription(data.Description).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		OnConflictColumns(entgroup.FieldName).
		Update(func(u *ent.GroupUpsert) {
			u.SetDisplayName(data.DisplayName)
			u.SetDescription(data.Description)
			u.SetEnvironment(data.Meta.Environment)
			u.SetNamespace(data.Meta.Namespace)
		}).
		ID(ctx)
	if err != nil {
		return fmt.Errorf("upsert group %q: %w", data.Name, err)
	}

	et, lk := cachekeys.Group(data.Name)
	r.cache.Set(et, lk, id)
	return nil
}

// Delete removes a Group entity from the database by name.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key GroupKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	deleted, err := r.client.Group.Delete().
		Where(entgroup.NameEQ(string(key))).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete group %q: %w", key, err)
	}
	if deleted > 0 {
		et, lk := cachekeys.Group(string(key))
		r.cache.Del(et, lk)
	}
	return nil
}
