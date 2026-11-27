// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apichangelog

import (
	"context"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entapi "github.com/telekom/controlplane/controlplane-api/ent/api"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

const entityType = "apichangelog"

// Repository stores the changelog file ID on the matching Api entity.
type Repository struct {
	client *ent.Client
}

var _ runtime.Repository[Key, *Data] = (*Repository)(nil)

// NewRepository creates a changelog repository.
func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

// Upsert sets the changelog file ID on the Api. If the Api has not been
// projected yet, it returns ErrDependencyMissing so the CR is retried.
// The resource name omits the version suffix of the base path, so a new major version reuses
// the name and retargets the changelog: the file ID is then cleared from the previous Api.
func (r *Repository) Upsert(ctx context.Context, data *Data) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	updated, err := r.client.Api.Update().
		Where(entapi.NamespaceEQ(data.Namespace), entapi.BasePathEQ(data.BasePath), entapi.HasOwnerWith(team.NameEQ(data.TeamName))).
		SetChangelog(data.FileID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("set changelog on api %q (team %q): %w", data.BasePath, data.TeamName, err)
	}
	if updated == 0 {
		return runtime.WrapDependencyMissing("api", data.BasePath)
	}

	// Done after the new assignment so a missing target Api keeps the old one until the retry.
	_, err = r.client.Api.Update().
		Where(entapi.NamespaceEQ(data.Namespace), entapi.BasePathNEQ(data.BasePath), entapi.HasOwnerWith(team.NameEQ(data.TeamName)), entapi.ChangelogEQ(data.FileID)).
		ClearChangelog().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("clear previous changelog assignment for %q (team %q): %w", data.BasePath, data.TeamName, err)
	}
	return nil
}

// Delete clears the changelog file ID. It is a no-op if the Api no longer exists.
func (r *Repository) Delete(ctx context.Context, key Key) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	_, err := r.client.Api.Update().
		Where(entapi.NamespaceEQ(key.Namespace), entapi.BasePathEQ(key.BasePath), entapi.HasOwnerWith(team.NameEQ(key.TeamName))).
		ClearChangelog().
		Save(ctx)
	if err != nil {
		return fmt.Errorf("clear changelog on api %q (team %q): %w", key.BasePath, key.TeamName, err)
	}
	return nil
}
