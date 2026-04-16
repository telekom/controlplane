// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package team

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/member"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for Team entities in the EdgeCache.
const entityType = "team"

// Repository performs typed persistence operations for Team entities.
// It implements runtime.Repository[TeamKey, *TeamData].
//
// Team has an optional FK dependency on Group (resolved via TeamDeps) and
// a nested entity pattern for Members. Member sync and orphan cleanup are
// performed within a database transaction.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   TeamDeps
	log    logr.Logger
}

// compile-time interface check.
var _ runtime.Repository[TeamKey, *TeamData] = (*Repository)(nil)

// NewRepository creates a Team repository wired with the given ent client,
// edge cache, dependency resolver, and logger.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps TeamDeps, log logr.Logger) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
		log:    log,
	}
}

// Upsert creates or updates a Team entity in the database within a
// transaction. It resolves the optional Group FK, upserts the Team row,
// upserts Members (keyed by email + team), and deletes orphaned Members.
//
// If the referenced Group does not exist, the Team is created without a
// Group FK and a warning is logged (non-fatal — Group FK is optional).
func (r *Repository) Upsert(ctx context.Context, data *TeamData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	// Resolve optional Group FK.
	groupID, err := r.deps.FindGroupID(ctx, data.GroupName)
	if err != nil {
		if !errors.Is(err, infrastructure.ErrEntityNotFound) {
			return fmt.Errorf("find group %q: %w", data.GroupName, err)
		}
		r.log.Info("group not found for team, proceeding without group FK",
			"team", data.Name, "group", data.GroupName, "error", err)
		groupID = 0
	}

	return r.withTx(ctx, func(tx *ent.Tx) error {
		// 1. Upsert the Team entity.
		create := tx.Team.Create().
			SetName(data.Name).
			SetEmail(data.Email).
			SetCategory(team.Category(data.Category)).
			SetStatusPhase(team.StatusPhase(data.StatusPhase)).
			SetStatusMessage(data.StatusMessage).
			SetEnvironment(data.Meta.Environment).
			SetNamespace(data.Meta.Namespace)

		if groupID > 0 {
			create = create.SetGroupID(groupID)
		}

		teamID, upsertErr := create.
			OnConflictColumns(team.FieldName).
			Update(func(u *ent.TeamUpsert) {
				u.SetEmail(data.Email)
				u.SetCategory(team.Category(data.Category))
				u.SetStatusPhase(team.StatusPhase(data.StatusPhase))
				u.SetStatusMessage(data.StatusMessage)
				u.SetEnvironment(data.Meta.Environment)
				u.SetNamespace(data.Meta.Namespace)
			}).
			ID(ctx)
		if upsertErr != nil {
			return fmt.Errorf("upsert team %q: %w", data.Name, upsertErr)
		}

		// 2. Sync members: upsert all current members and collect their IDs.
		currentMemberIDs, memberErr := syncMembers(ctx, tx, teamID, data.Members)
		if memberErr != nil {
			return memberErr
		}

		// 3. Delete orphaned members no longer in the spec.
		if deleteErr := deleteOrphanedMembers(ctx, tx, teamID, currentMemberIDs); deleteErr != nil {
			return deleteErr
		}

		et, lk := cachekeys.Team(data.Name)
		r.cache.Set(et, lk, teamID)
		return nil
	})
}

// Delete removes a Team entity from the database by name.
// Explicitly deletes associated Members before removing the Team row,
// since ent does not configure ON DELETE CASCADE by default.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key TeamKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	name := string(key)

	// Look up the team; if not found, nothing to delete.
	t, err := r.client.Team.Query().Where(team.NameEQ(name)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("find team %q for delete: %w", name, err)
	}

	// Delete child Members first.
	if _, err := r.client.Member.Delete().
		Where(member.HasTeamWith(team.IDEQ(t.ID))).
		Exec(ctx); err != nil {
		return fmt.Errorf("delete members for team %q: %w", name, err)
	}

	// Delete the Team itself.
	if err := r.client.Team.DeleteOneID(t.ID).Exec(ctx); err != nil {
		return fmt.Errorf("delete team %q: %w", name, err)
	}

	et, lk := cachekeys.Team(name)
	r.cache.Del(et, lk)
	return nil
}

// withTx runs fn inside a database transaction, handling commit/rollback.
func (r *Repository) withTx(ctx context.Context, fn func(tx *ent.Tx) error) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %w (original: %w)", rbErr, err)
		}
		return err
	}
	return tx.Commit()
}

// syncMembers upserts all current members for a team and returns their IDs.
// Members are keyed by the composite unique index (email, team_members), so
// an existing member with the same email in the same team is updated in place
// rather than recreated. This avoids ID churn and sequence-related issues.
func syncMembers(ctx context.Context, tx *ent.Tx, teamID int, members []MemberData) ([]int, error) {
	ids := make([]int, 0, len(members))
	for _, m := range members {
		id, err := tx.Member.Create().
			SetName(m.Name).
			SetEmail(m.Email).
			SetTeamID(teamID).
			OnConflictColumns(member.FieldEmail, member.TeamColumn).
			Update(func(u *ent.MemberUpsert) {
				u.SetName(m.Name)
			}).
			ID(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert member %q: %w", m.Name, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// deleteOrphanedMembers removes any members belonging to teamID whose IDs
// are NOT in the currentIDs set.
func deleteOrphanedMembers(ctx context.Context, tx *ent.Tx, teamID int, currentIDs []int) error {
	_, err := tx.Member.Delete().
		Where(
			member.HasTeamWith(team.IDEQ(teamID)),
			member.IDNotIn(currentIDs...),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete orphaned members for team %d: %w", teamID, err)
	}
	return nil
}
