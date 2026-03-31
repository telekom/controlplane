// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"

	"github.com/spf13/viper"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Stores is the dependency container holding all ObjectStore instances
// needed by the spy-server controllers and mappers.
type Stores struct {
	APIExposureStore     store.ObjectStore[*apiv1.ApiExposure]
	APISubscriptionStore store.ObjectStore[*apiv1.ApiSubscription]
	ApplicationStore     store.ObjectStore[*applicationv1.Application]
	ZoneStore            store.ObjectStore[*adminv1.Zone]
	ApprovalStore        store.ObjectStore[*approvalv1.Approval]

	EventExposureStore     store.ObjectStore[*eventv1.EventExposure]
	EventSubscriptionStore store.ObjectStore[*eventv1.EventSubscription]
	EventTypeStore         store.ObjectStore[*eventv1.EventType]
}

// NewStores creates and initialises all stores. It panics if any store
// cannot be created (same semantics as rover-server).
func NewStores(ctx context.Context, cfg *rest.Config) *Stores {
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	return &Stores{
		APIExposureStore:     NewOrDie[*apiv1.ApiExposure](ctx, dynamicClient, apiv1.GroupVersion.WithResource("apiexposures"), apiv1.GroupVersion.WithKind("ApiExposure")),
		APISubscriptionStore: NewOrDie[*apiv1.ApiSubscription](ctx, dynamicClient, apiv1.GroupVersion.WithResource("apisubscriptions"), apiv1.GroupVersion.WithKind("ApiSubscription")),
		ApplicationStore:     NewOrDie[*applicationv1.Application](ctx, dynamicClient, applicationv1.GroupVersion.WithResource("applications"), applicationv1.GroupVersion.WithKind("Application")),
		ZoneStore:            NewOrDie[*adminv1.Zone](ctx, dynamicClient, adminv1.GroupVersion.WithResource("zones"), adminv1.GroupVersion.WithKind("Zone")),
		ApprovalStore:        NewOrDie[*approvalv1.Approval](ctx, dynamicClient, approvalv1.GroupVersion.WithResource("approvals"), approvalv1.GroupVersion.WithKind("Approval")),

		EventExposureStore:     NewOrDie[*eventv1.EventExposure](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventexposures"), eventv1.GroupVersion.WithKind("EventExposure")),
		EventSubscriptionStore: NewOrDie[*eventv1.EventSubscription](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventsubscriptions"), eventv1.GroupVersion.WithKind("EventSubscription")),
		EventTypeStore:         NewOrDie[*eventv1.EventType](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventtypes"), eventv1.GroupVersion.WithKind("EventType")),
	}
}

func NewOrDie[T store.Object](ctx context.Context, dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind) store.ObjectStore[T] {
	storeOpts := inmemory.StoreOpts{
		Client:       dynamicClient,
		GVR:          gvr,
		GVK:          gvk,
		AllowedSorts: []string{},
		Database: inmemory.DatabaseOpts{
			Filepath:     viper.GetString("database.filepath"),
			ReduceMemory: viper.GetBool("database.reduceMemory"),
		},
		Informer: inmemory.InformerOpts{
			DisableCache: viper.GetBool("informer.disableCache"),
		},
	}

	return inmemory.NewSortableOrDie[T](ctx, storeOpts)
}
