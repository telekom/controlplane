// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/telekom/controlplane/controlplane-api/ent"
	entlistener "github.com/telekom/controlplane/controlplane-api/ent/listener"
	"github.com/telekom/controlplane/projector/internal/domain/shared"
	"github.com/telekom/controlplane/projector/internal/infrastructure"
	"github.com/telekom/controlplane/projector/internal/metrics"
	"github.com/telekom/controlplane/projector/internal/runtime"
)

type Dependencies interface {
	FindApplicationByMeta(context.Context, string, string) (int, error)
	FindAPISubscriptionID(context.Context, string, string, string) (int, error)
	FindAPIExposureID(context.Context, string, string, string) (int, error)
	EvictApplicationByMeta(string, string)
	EvictAPISubscriptionID(string, string, string)
	EvictAPIExposureID(string, string, string)
}

type Repository struct {
	client *ent.Client
	deps   Dependencies
}

var _ runtime.Repository[Key, *Data] = (*Repository)(nil)

func NewRepository(client *ent.Client, deps Dependencies) *Repository {
	return &Repository{client: client, deps: deps}
}

func (r *Repository) Upsert(ctx context.Context, data *Data) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues("listener", metrics.OperationUpsert).Observe(time.Since(start).Seconds())
	}()

	observerID, err := r.deps.FindApplicationByMeta(ctx, data.Observer.Namespace, data.Observer.Name)
	if err != nil {
		return r.handleDependencyError(ctx, data, "application", data.Observer.Namespace+"/"+data.Observer.Name, err)
	}
	consumerTeam := shared.TeamNameFromNamespace(data.Consumer.Namespace)
	subID, err := r.deps.FindAPISubscriptionID(ctx, data.BasePath, data.Consumer.Name, consumerTeam)
	if err != nil {
		return r.handleDependencyError(ctx, data, "api_subscription", data.Consumer.Namespace+"/"+data.Consumer.Name+data.BasePath, err)
	}
	providerTeam := shared.TeamNameFromNamespace(data.Provider.Namespace)
	exposureID, err := r.deps.FindAPIExposureID(ctx, data.BasePath, data.Provider.Name, providerTeam)
	if err != nil {
		return r.handleDependencyError(ctx, data, "api_exposure", data.Provider.Namespace+"/"+data.Provider.Name+data.BasePath, err)
	}

	// Both the insert and the edge update are atomic; a failed retarget must not leave old edges.
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin listener transaction: %w", err)
	}
	create := tx.Listener.Create().
		SetNamespace(data.Meta.Namespace).
		SetName(data.Meta.Name).
		SetEnvironment(data.Meta.Environment).
		SetStatusPhase(entlistener.StatusPhase(data.StatusPhase)).
		SetStatusMessage(data.StatusMessage).
		SetAPIBasePath(data.BasePath).
		SetApplicationID(observerID).
		SetSubscriptionID(subID).
		SetExposureID(exposureID)
	if data.RequestFilter != nil {
		create.SetRequestFilter(data.RequestFilter)
	}
	if data.ResponseFilter != nil {
		create.SetResponseFilter(data.ResponseFilter)
	}
	id, err := create.OnConflictColumns(entlistener.FieldNamespace, entlistener.FieldName).UpdateNewValues().ID(ctx)
	if err == nil {
		update := tx.Listener.UpdateOneID(id).
			SetApplicationID(observerID).
			SetSubscriptionID(subID).
			SetExposureID(exposureID)
		if data.RequestFilter == nil {
			update.ClearRequestFilter()
		}
		if data.ResponseFilter == nil {
			update.ClearResponseFilter()
		}
		err = update.Exec(ctx)
	}
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("upsert listener %s/%s: %w (rollback: %v)", data.Meta.Namespace, data.Meta.Name, err, rollbackErr)
		}
		if infrastructure.IsFKViolation(err, "") {
			r.deps.EvictApplicationByMeta(data.Observer.Namespace, data.Observer.Name)
			r.deps.EvictAPISubscriptionID(data.BasePath, data.Consumer.Name, consumerTeam)
			r.deps.EvictAPIExposureID(data.BasePath, data.Provider.Name, providerTeam)
			return runtime.WrapDependencyMissing("listener edge", data.Meta.Namespace+"/"+data.Meta.Name)
		}
		return fmt.Errorf("upsert listener %s/%s: %w", data.Meta.Namespace, data.Meta.Name, err)
	}
	if err := tx.Commit(); err != nil {
		r.deps.EvictApplicationByMeta(data.Observer.Namespace, data.Observer.Name)
		r.deps.EvictAPISubscriptionID(data.BasePath, data.Consumer.Name, consumerTeam)
		r.deps.EvictAPIExposureID(data.BasePath, data.Provider.Name, providerTeam)
		return fmt.Errorf("commit listener %s/%s: %w", data.Meta.Namespace, data.Meta.Name, err)
	}
	return nil
}

func (r *Repository) handleDependencyError(ctx context.Context, data *Data, kind, key string, err error) error {
	if !errors.Is(err, infrastructure.ErrEntityNotFound) {
		return fmt.Errorf("find %s %s: %w", kind, key, err)
	}
	// A retargeted Listener must not remain visible with its previous owner edges.
	if err := r.Delete(ctx, Key{Namespace: data.Meta.Namespace, Name: data.Meta.Name}); err != nil {
		return fmt.Errorf("remove stale listener before retry: %w", err)
	}
	return runtime.WrapDependencyMissing(kind, key)
}

func (r *Repository) Delete(ctx context.Context, key Key) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues("listener", metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()
	_, err := r.client.Listener.Delete().Where(entlistener.NamespaceEQ(key.Namespace), entlistener.NameEQ(key.Name)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete listener %s/%s: %w", key.Namespace, key.Name, err)
	}
	return nil
}
