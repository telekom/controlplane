// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"errors"
	"fmt"
	"time"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entapproval "github.com/telekom/controlplane/controlplane-api/ent/approval"
	entlistener "github.com/telekom/controlplane/controlplane-api/ent/listener"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/infrastructure/cachekeys"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

// entityType is the cache key prefix for Approval entities in the EdgeCache.
const entityType = "approval"

// Repository performs typed persistence operations for Approval entities.
// It implements runtime.Repository[ApprovalKey, *ApprovalData].
//
// Approval has a required FK dependency on ApiSubscription, EventSubscription,
// AgenticSubscription, FileSubscription, or Listener, determined by the
// TargetKind field in ApprovalData. On conflict, UpdateNewValues() updates the
// active edge FK column from the INSERT statement while the chained Update()
// callback NULLs the inactive target FK columns to handle target re-targeting
// in a single atomic statement. A single cache entry keyed by namespace:name is
// maintained.
type Repository struct {
	client *ent.Client
	cache  *infrastructure.EdgeCache
	deps   ApprovalDeps
}

// compile-time interface check.
var _ runtime.Repository[ApprovalKey, *ApprovalData] = (*Repository)(nil)

// NewRepository creates an Approval repository wired with the given
// ent client, edge cache, and dependency resolver.
func NewRepository(client *ent.Client, cache *infrastructure.EdgeCache, deps ApprovalDeps) *Repository {
	return &Repository{
		client: client,
		cache:  cache,
		deps:   deps,
	}
}

// resolveSubscriptionID resolves the parent subscription FK based on the
// target kind (ApiSubscription, EventSubscription, AgenticSubscription, FileSubscription, or Listener).
func (r *Repository) resolveSubscriptionID(ctx context.Context, data *ApprovalData) (int, error) {
	switch data.TargetKind {
	case TargetKindListener:
		if err := shared.ValidateListenerGate(data.ApprovalKey, data.Action); err != nil {
			return 0, err
		}
		id, err := r.client.Listener.Query().
			Where(entlistener.NamespaceEQ(data.SubscriptionNamespace), entlistener.NameEQ(data.SubscriptionName)).
			OnlyID(ctx)
		if ent.IsNotFound(err) {
			return 0, runtime.WrapDependencyMissing("listener", data.SubscriptionNamespace+"/"+data.SubscriptionName)
		}
		if err != nil {
			return 0, fmt.Errorf("find listener %s/%s: %w", data.SubscriptionNamespace, data.SubscriptionName, err)
		}
		return id, nil
	case TargetKindEventSubscription:
		id, err := r.deps.FindEventSubscriptionByMeta(ctx, data.SubscriptionNamespace, data.SubscriptionName)
		if err != nil {
			if errors.Is(err, infrastructure.ErrEntityNotFound) {
				return 0, runtime.WrapDependencyMissing("event_subscription",
					data.SubscriptionNamespace+"/"+data.SubscriptionName)
			}
			return 0, fmt.Errorf("find event_subscription %s/%s: %w",
				data.SubscriptionNamespace, data.SubscriptionName, err)
		}
		return id, nil
	case TargetKindAgenticSubscription:
		id, err := r.deps.FindAgenticSubscriptionByMeta(ctx, data.SubscriptionNamespace, data.SubscriptionName)
		if err != nil {
			if errors.Is(err, infrastructure.ErrEntityNotFound) {
				return 0, runtime.WrapDependencyMissing("agentic_subscription",
					data.SubscriptionNamespace+"/"+data.SubscriptionName)
			}
			return 0, fmt.Errorf("find agentic_subscription %s/%s: %w",
				data.SubscriptionNamespace, data.SubscriptionName, err)
		}
		return id, nil
	case TargetKindFileSubscription:
		id, err := r.deps.FindFileSubscriptionByMeta(ctx, data.SubscriptionNamespace, data.SubscriptionName)
		if err != nil {
			if errors.Is(err, infrastructure.ErrEntityNotFound) {
				return 0, runtime.WrapDependencyMissing("file_subscription",
					data.SubscriptionNamespace+"/"+data.SubscriptionName)
			}
			return 0, fmt.Errorf("find file_subscription %s/%s: %w",
				data.SubscriptionNamespace, data.SubscriptionName, err)
		}
		return id, nil
	default: // TargetKindAPISubscription
		id, err := r.deps.FindAPISubscriptionByMeta(ctx, data.SubscriptionNamespace, data.SubscriptionName)
		if err != nil {
			if errors.Is(err, infrastructure.ErrEntityNotFound) {
				return 0, runtime.WrapDependencyMissing("api_subscription",
					data.SubscriptionNamespace+"/"+data.SubscriptionName)
			}
			return 0, fmt.Errorf("find api_subscription %s/%s: %w",
				data.SubscriptionNamespace, data.SubscriptionName, err)
		}
		return id, nil
	}
}

// Upsert creates or updates an Approval entity in the database.
//
// Steps:
//  1. Resolve parent subscription FK (required) via cache-based meta-key
//     lookup, branching on TargetKind. Returns ErrDependencyMissing if the
//     subscription has not been synced yet.
//  2. Upsert in a single statement with ON CONFLICT (namespace, name),
//     UpdateNewValues() (which updates the active target FK column included in
//     the INSERT), and a chained Update() callback that sets the inactive
//     target FK columns to NULL.
//  3. Write cache entry keyed by namespace:name.
func (r *Repository) Upsert(ctx context.Context, data *ApprovalData) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	subID, err := r.resolveSubscriptionID(ctx, data)
	if err != nil {
		return err
	}

	create := r.client.Approval.Create().
		SetNamespace(data.Meta.Namespace).
		SetName(data.Meta.Name).
		SetAction(data.Action).
		SetStrategy(entapproval.Strategy(data.Strategy)).
		SetState(entapproval.State(data.State)).
		SetStatusPhase(entapproval.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetEnvironment(data.Meta.Environment).
		SetRequester(data.Requester).
		SetDecider(data.Decider).
		SetDeciderTeamName(data.Decider.TeamName).
		SetDecisions(data.Decisions).
		SetAvailableTransitions(data.AvailableTransitions).
		SetNillableExpiresAt(data.ExpiresAt).
		SetRequestedScopes(data.AccessScopes)

	// Set the active target FK column based on target kind.
	var activeColumn string
	switch data.TargetKind {
	case TargetKindListener:
		if data.ApprovalKey != approvalv1.ApprovalKeyProvider {
			create = create.SetConsumerListenerID(subID)
			activeColumn = entapproval.ConsumerListenerColumn
		} else {
			create = create.SetListenerID(subID)
			activeColumn = entapproval.ListenerColumn
		}
	case TargetKindEventSubscription:
		create = create.SetEventSubscriptionID(subID)
		activeColumn = entapproval.EventSubscriptionColumn
	case TargetKindAgenticSubscription:
		create = create.SetAgenticSubscriptionID(subID)
		activeColumn = entapproval.AgenticSubscriptionColumn
	case TargetKindFileSubscription:
		create = create.SetFileSubscriptionID(subID)
		activeColumn = entapproval.FileSubscriptionColumn
	default:
		create = create.SetAPISubscriptionID(subID)
		activeColumn = entapproval.APISubscriptionColumn
	}

	targetFKColumns := []string{
		entapproval.APISubscriptionColumn,
		entapproval.EventSubscriptionColumn,
		entapproval.AgenticSubscriptionColumn,
		entapproval.FileSubscriptionColumn,
		entapproval.ListenerColumn,
		entapproval.ConsumerListenerColumn,
	}

	approvalID, upsertErr := create.
		OnConflictColumns(entapproval.FieldNamespace, entapproval.FieldName).
		UpdateNewValues().
		Update(func(u *ent.ApprovalUpsert) {
			for _, col := range targetFKColumns {
				if col != activeColumn {
					u.SetNull(col)
				}
			}
		}).
		ID(ctx)
	if upsertErr != nil {
		if infrastructure.IsFKViolation(upsertErr, "") {
			r.evictSubscriptionCache(data)
			return runtime.WrapDependencyMissing(dependencyKind(data.TargetKind),
				data.SubscriptionNamespace+"/"+data.SubscriptionName)
		}
		return fmt.Errorf("upsert approval %s/%s (sub %s/%s): %w",
			data.Meta.Namespace, data.Meta.Name,
			data.SubscriptionNamespace, data.SubscriptionName, upsertErr)
	}

	et, lk := cachekeys.Approval(data.Meta.Namespace, data.Meta.Name)
	r.cache.Set(et, lk, approvalID)
	return nil
}

// dependencyKind maps a TargetKind to the lowercase dependency-name label
// used in ErrDependencyMissing error messages.
func dependencyKind(targetKind string) string {
	switch targetKind {
	case TargetKindListener:
		return "listener"
	case TargetKindEventSubscription:
		return "event_subscription"
	case TargetKindAgenticSubscription:
		return "agentic_subscription"
	case TargetKindFileSubscription:
		return "file_subscription"
	default:
		return "api_subscription"
	}
}

// evictSubscriptionCache removes the stale cached subscription ID so the
// next reconcile attempt performs a fresh DB lookup.
func (r *Repository) evictSubscriptionCache(data *ApprovalData) {
	switch data.TargetKind {
	case TargetKindListener:
		return
	case TargetKindEventSubscription:
		r.deps.EvictEventSubscription(data.SubscriptionNamespace, data.SubscriptionName)
	case TargetKindAgenticSubscription:
		r.deps.EvictAgenticSubscription(data.SubscriptionNamespace, data.SubscriptionName)
	case TargetKindFileSubscription:
		r.deps.EvictFileSubscription(data.SubscriptionNamespace, data.SubscriptionName)
	default:
		r.deps.EvictAPISubscription(data.SubscriptionNamespace, data.SubscriptionName)
	}
}

// Delete removes an Approval entity from the database by its Kubernetes
// object identity (namespace + name). Also cleans the cache entry if the
// entity was found.
// Returns nil if the entity does not exist (idempotent delete).
func (r *Repository) Delete(ctx context.Context, key ApprovalKey) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues(entityType, metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()

	count, err := r.client.Approval.Delete().
		Where(
			entapproval.NamespaceEQ(key.Namespace),
			entapproval.NameEQ(key.Name),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete approval %s/%s: %w", key.Namespace, key.Name, err)
	}
	if count > 0 {
		et, lk := cachekeys.Approval(key.Namespace, key.Name)
		r.cache.Del(et, lk)
	}
	return nil
}
