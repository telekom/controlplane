// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filetype

import (
	"context"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entfiletype "github.com/telekom/controlplane/controlplane-api/ent/filetype"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for FileType entities in the EdgeCache.
const entityType = "filetype"

// Repository performs typed persistence operations for FileType catalogue entities.
// It implements runtime.Repository[FileTypeKey, *FileTypeData].
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
}

// compile-time interface check.
var _ runtime.Repository[FileTypeKey, *FileTypeData] = (*Repository)(nil)

// NewRepository creates a FileType repository wired with the given ent client,
// edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
	}
}

// Upsert creates or updates a FileType catalogue entity in the database.
func (r *Repository) Upsert(ctx context.Context, data *FileTypeData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	create := r.client.FileType.Create().
		SetFileType(data.FileType).
		SetDescription(data.Description).
		SetActive(data.Active).
		SetStatusPhase(entfiletype.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetNamespace(data.Meta.Namespace)

	if data.Variant != nil {
		create.SetVariant(*data.Variant)
	}

	fileTypeID, upsertErr := create.
		OnConflictColumns(entfiletype.FieldFileType).
		Update(func(u *ent.FileTypeUpsert) {
			u.SetDescription(data.Description)
			u.SetActive(data.Active)
			u.SetStatusPhase(entfiletype.StatusPhase(data.StatusPhase))
			u.SetStatusMessage(data.StatusMessage)
			u.SetNamespace(data.Meta.Namespace)
			if data.Variant != nil {
				u.SetVariant(*data.Variant)
			} else {
				u.ClearVariant()
			}
		}).
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert file_type %q: %w", data.FileType, upsertErr)
	}

	et, lk := cachekeys.FileTypeDef(data.FileType)
	r.cache.Set(et, lk, fileTypeID)
	if data.Active {
		aet, alk := cachekeys.ActiveFileType(data.FileType)
		r.cache.Set(aet, alk, fileTypeID)
	} else {
		aet, alk := cachekeys.ActiveFileType(data.FileType)
		r.cache.Del(aet, alk)
	}
	return nil
}

// Delete removes a FileType catalogue entity from the database by file type.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key FileTypeKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.FileType.Delete().
		Where(
			entfiletype.FileTypeEQ(key.FileType),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete file_type %q: %w", key.FileType, err)
	}
	if count > 0 {
		et, lk := cachekeys.FileTypeDef(key.FileType)
		r.cache.Del(et, lk)
		aet, alk := cachekeys.ActiveFileType(key.FileType)
		r.cache.Del(aet, alk)
	}
	return nil
}
