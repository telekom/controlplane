// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package filesubscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	entfilesubscription "github.com/telekom/controlplane/controlplane-api/ent/filesubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for FileSubscription entities in the EdgeCache.
const entityType = "filesubscription"

// Repository performs typed persistence operations for FileSubscription entities.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   FileSubscriptionDeps
}

// compile-time interface check.
var _ runtime.Repository[FileSubscriptionKey, *FileSubscriptionData] = (*Repository)(nil)

// NewRepository creates a FileSubscription repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps FileSubscriptionDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// Upsert creates or updates a FileSubscription entity in the database.
func (r *Repository) Upsert(ctx context.Context, data *FileSubscriptionData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	ownerAppID, err := r.deps.FindApplicationID(ctx, data.OwnerAppName, data.OwnerTeamName)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("application", data.OwnerAppName)
		}
		return fmt.Errorf("find application %q (team %q): %w", data.OwnerAppName, data.OwnerTeamName, err)
	}

	zoneID, err := r.deps.FindZoneID(ctx, data.Zone)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("zone", data.Zone)
		}
		return fmt.Errorf("find zone %q: %w", data.Zone, err)
	}

	var fileTypeDefID *int
	if id, findErr := r.deps.FindFileTypeID(ctx, data.TargetFileType); findErr != nil {
		if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find file_type %q: %w", data.TargetFileType, findErr)
		}
	} else {
		fileTypeDefID = &id
	}

	var targetExposureID *int
	if id, findErr := r.deps.FindActiveFileExposureByFileType(ctx, data.TargetFileType); findErr != nil {
		if !errors.Is(findErr, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find active file_exposure %q: %w", data.TargetFileType, findErr)
		}
	} else {
		targetExposureID = &id
	}

	create := r.client.FileSubscription.Create().
		SetFileType(data.TargetFileType).
		SetZoneName(data.Zone).
		SetSftpPublicKeys(data.SFTPPublicKeys).
		SetStatusPhase(entfilesubscription.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetNamespace(data.Meta.Namespace).
		SetName(data.Meta.Name).
		SetOwnerID(ownerAppID).
		SetZoneID(zoneID).
		SetNillableFileTypeDefID(fileTypeDefID).
		SetNillableTargetID(targetExposureID)

	subscriptionID, upsertErr := create.
		OnConflictColumns(entfilesubscription.FieldFileType, entfilesubscription.OwnerColumn).
		Update(func(u *ent.FileSubscriptionUpsert) {
			u.SetZoneName(data.Zone)
			u.UpdateSftpPublicKeys()
			u.SetStatusPhase(entfilesubscription.StatusPhase(data.StatusPhase))
			u.SetStatusMessage(data.StatusMessage)
			u.SetEnvironment(data.Meta.Environment)
			u.SetNamespace(data.Meta.Namespace)
			u.SetName(data.Meta.Name)
		}).
		ID(ctx)
	if upsertErr != nil {
		return fmt.Errorf("upsert file_subscription (owner %q, fileType %q): %w", data.OwnerAppName, data.TargetFileType, upsertErr)
	}

	// Edge FKs are not part of ent upsert SET clauses; update them explicitly.
	update := r.client.FileSubscription.UpdateOneID(subscriptionID).SetZoneID(zoneID)
	if fileTypeDefID != nil {
		update = update.SetFileTypeDefID(*fileTypeDefID)
	} else {
		update = update.ClearFileTypeDef()
	}
	if targetExposureID != nil {
		update = update.SetTargetID(*targetExposureID)
	} else {
		update = update.ClearTarget()
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("update edge FKs for file_subscription %d: %w", subscriptionID, err)
	}

	et, lk := cachekeys.FileSubscriptionMeta(data.Meta.Namespace, data.Meta.Name)
	r.cache.Set(et, lk, subscriptionID)
	return nil
}

// Delete removes a FileSubscription entity from the database by owner
// application name, team name, and file type.
func (r *Repository) Delete(ctx context.Context, key FileSubscriptionKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.FileSubscription.Delete().
		Where(
			entfilesubscription.HasOwnerWith(
				application.NameEQ(key.OwnerAppName),
				application.HasOwnerTeamWith(team.NameEQ(key.OwnerTeamName)),
			),
			entfilesubscription.FileTypeEQ(key.FileType),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete file_subscription (owner %q, team %q, fileType %q): %w",
			key.OwnerAppName, key.OwnerTeamName, key.FileType, err)
	}
	if count > 0 {
		if key.Namespace != "" && key.Name != "" {
			et, lk := cachekeys.FileSubscriptionMeta(key.Namespace, key.Name)
			r.cache.Del(et, lk)
		}
	}
	return nil
}
