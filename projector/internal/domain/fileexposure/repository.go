// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package fileexposure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	entfileexposure "github.com/telekom/controlplane/controlplane-api/ent/fileexposure"
	entfilesubscription "github.com/telekom/controlplane/controlplane-api/ent/filesubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for FileExposure entities in the EdgeCache.
const entityType = "fileexposure"

// Repository performs typed persistence operations for FileExposure entities.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   FileExposureDeps
}

// compile-time interface check.
var _ runtime.Repository[FileExposureKey, *FileExposureData] = (*Repository)(nil)

// NewRepository creates a FileExposure repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps FileExposureDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates a FileExposure entity in the database.
func (r *Repository) Upsert(ctx context.Context, data *FileExposureData) error {
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

	zoneID, err := r.deps.FindZoneID(ctx, data.ZoneName)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("zone", data.ZoneName)
		}
		return fmt.Errorf("find zone %q: %w", data.ZoneName, err)
	}

	var fileTypeDefID *int
	id, findErr := r.deps.FindFileTypeID(ctx, data.TargetFileType)
	if findErr != nil {
		if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find file_type %q: %w", data.TargetFileType, findErr)
		}
	} else {
		fileTypeDefID = &id
	}

	create := r.client.FileExposure.Create().
		SetFileType(data.TargetFileType).
		SetVisibility(entfileexposure.Visibility(data.Visibility)).
		SetActive(data.Active).
		SetZoneName(data.ZoneName).
		SetSftpPublicKeys(data.SFTPPublicKeys).
		SetApprovalConfig(data.ApprovalConfig).
		SetStatusPhase(entfileexposure.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetOwnerID(appID).
		SetZoneID(zoneID).
		SetNillableFileTypeDefID(fileTypeDefID)

	if data.Provider != nil {
		create.SetProvider(*data.Provider)
	}
	if data.ZoneNamespace != nil {
		create.SetZoneNamespace(*data.ZoneNamespace)
	}

	exposureID, upsertErr := create.
		OnConflictColumns(entfileexposure.FieldFileType, entfileexposure.OwnerColumn).
		Update(func(u *ent.FileExposureUpsert) {
			u.SetVisibility(entfileexposure.Visibility(data.Visibility))
			u.SetActive(data.Active)
			u.SetZoneName(data.ZoneName)
			u.UpdateSftpPublicKeys()
			u.UpdateApprovalConfig()
			u.SetStatusPhase(entfileexposure.StatusPhase(data.StatusPhase))
			u.SetStatusMessage(data.StatusMessage)
			u.SetEnvironment(data.Meta.Environment)
			u.SetNamespace(data.Meta.Namespace)
			if data.Provider != nil {
				u.SetProvider(*data.Provider)
			} else {
				u.ClearProvider()
			}
			if data.ZoneNamespace != nil {
				u.SetZoneNamespace(*data.ZoneNamespace)
			} else {
				u.ClearZoneNamespace()
			}
		}).
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert file_exposure %q (app %q, team %q): %w", data.TargetFileType, data.AppName, data.TeamName, upsertErr)
	}

	// Edge FKs are not part of ent upsert SET clauses; update them explicitly.
	update := r.client.FileExposure.UpdateOneID(exposureID).SetZoneID(zoneID)
	if fileTypeDefID != nil {
		update = update.SetFileTypeDefID(*fileTypeDefID)
	} else {
		update = update.ClearFileTypeDef()
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("update edge FKs for file_exposure %d: %w", exposureID, err)
	}

	et, lk := cachekeys.FileExposure(data.TargetFileType, data.AppName, data.TeamName)
	r.cache.Set(et, lk, exposureID)

	// Back-link subscriptions that were projected before this exposure existed.
	// FileSubscription resolves its optional target by active FileExposure + file type.
	// If subscriptions were stored first, they can remain with NULL target until
	// this exposure appears.
	if data.Active {
		if err := r.client.FileSubscription.Update().
			Where(
				entfilesubscription.FileTypeEQ(data.TargetFileType),
				entfilesubscription.Not(entfilesubscription.HasTarget()),
			).
			SetTargetID(exposureID).
			Exec(ctx); err != nil {
			return fmt.Errorf("back-link subscriptions to file_exposure %q (app %q, team %q): %w",
				data.TargetFileType, data.AppName, data.TeamName, err)
		}
	}

	if data.Active {
		aet, alk := cachekeys.ActiveFileExposure(data.TargetFileType)
		r.cache.Set(aet, alk, exposureID)
	} else {
		aet, alk := cachekeys.ActiveFileExposure(data.TargetFileType)
		r.cache.Del(aet, alk)
	}
	return nil
}

// Delete removes a FileExposure entity from the database by file type,
// application name, and team name.
func (r *Repository) Delete(ctx context.Context, key FileExposureKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.FileExposure.Delete().
		Where(
			entfileexposure.FileTypeEQ(key.FileType),
			entfileexposure.HasOwnerWith(
				application.NameEQ(key.AppName),
				application.HasOwnerTeamWith(team.NameEQ(key.TeamName)),
			),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete file_exposure %q (app %q, team %q): %w", key.FileType, key.AppName, key.TeamName, err)
	}
	if count > 0 {
		et, lk := cachekeys.FileExposure(key.FileType, key.AppName, key.TeamName)
		r.cache.Del(et, lk)
		aet, alk := cachekeys.ActiveFileExposure(key.FileType)
		r.cache.Del(aet, alk)
	}
	return nil
}
