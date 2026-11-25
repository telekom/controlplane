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
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("application", data.Observer.Namespace+"/"+data.Observer.Name)
		}
		return fmt.Errorf("find application %s/%s: %w", data.Observer.Namespace, data.Observer.Name, err)
	}
	consumerTeam := shared.TeamNameFromNamespace(data.Consumer.Namespace)
	subID, err := r.deps.FindAPISubscriptionID(ctx, data.BasePath, data.Consumer.Name, consumerTeam)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("api_subscription", data.Consumer.Namespace+"/"+data.Consumer.Name+data.BasePath)
		}
		return fmt.Errorf("find api_subscription %s/%s%s: %w", data.Consumer.Namespace, data.Consumer.Name, data.BasePath, err)
	}
	providerTeam := shared.TeamNameFromNamespace(data.Provider.Namespace)
	exposureID, err := r.deps.FindAPIExposureID(ctx, data.BasePath, data.Provider.Name, providerTeam)
	if err != nil {
		if errors.Is(err, infrastructure.ErrEntityNotFound) {
			return runtime.WrapDependencyMissing("api_exposure", data.Provider.Namespace+"/"+data.Provider.Name+data.BasePath)
		}
		return fmt.Errorf("find api_exposure %s/%s%s: %w", data.Provider.Namespace, data.Provider.Name, data.BasePath, err)
	}

	create := r.client.Listener.Create().
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
	err = create.
		OnConflictColumns(entlistener.FieldNamespace, entlistener.FieldName).
		UpdateNewValues().
		Update(func(u *ent.ListenerUpsert) {
			if data.RequestFilter == nil {
				u.ClearRequestFilter()
			}
			if data.ResponseFilter == nil {
				u.ClearResponseFilter()
			}
		}).
		Exec(ctx)
	if err != nil {
		if infrastructure.IsFKViolation(err, "") {
			r.deps.EvictApplicationByMeta(data.Observer.Namespace, data.Observer.Name)
			r.deps.EvictAPISubscriptionID(data.BasePath, data.Consumer.Name, consumerTeam)
			r.deps.EvictAPIExposureID(data.BasePath, data.Provider.Name, providerTeam)
			return runtime.WrapDependencyMissing("listener edge", data.Meta.Namespace+"/"+data.Meta.Name)
		}
		return fmt.Errorf("upsert listener %s/%s: %w", data.Meta.Namespace, data.Meta.Name, err)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, key Key) error {
	start := time.Now()
	defer func() {
		metrics.DBOperationDuration.WithLabelValues("listener", metrics.OperationDelete).Observe(time.Since(start).Seconds())
	}()
	// Namespace and name identify exactly one row; namespaces are environment-prefixed.
	_, err := r.client.Listener.Delete().Where(entlistener.NamespaceEQ(key.Namespace), entlistener.NameEQ(key.Name)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete listener %s/%s: %w", key.Namespace, key.Name, err)
	}
	return nil
}
