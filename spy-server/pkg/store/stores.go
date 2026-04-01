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
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Stores is the dependency container holding all ObjectStore instances
// needed by the spy-server controllers and mappers.
type Stores struct {
	APIExposureStore       store.ObjectStore[*apiv1.ApiExposure]
	APIExposureSecretStore store.ObjectStore[*apiv1.ApiExposure]

	APISubscriptionStore       store.ObjectStore[*apiv1.ApiSubscription]
	APISubscriptionSecretStore store.ObjectStore[*apiv1.ApiSubscription]

	ApplicationStore store.ObjectStore[*applicationv1.Application]
	ZoneStore        store.ObjectStore[*adminv1.Zone]
	ApprovalStore    store.ObjectStore[*approvalv1.Approval]

	EventExposureStore     store.ObjectStore[*eventv1.EventExposure]
	EventSubscriptionStore store.ObjectStore[*eventv1.EventSubscription]
	EventTypeStore         store.ObjectStore[*eventv1.EventType]
}

// SecretsForKinds maps CRD kind names to the JSON paths of their secret fields.
var SecretsForKinds = map[string][]string{
	"ApiSubscription": {
		"spec.security.m2m.client.clientSecret",
		"spec.security.m2m.basic.password",
	},
	"ApiExposure": {
		"spec.security.m2m.externalIDP.client.clientSecret",
		"spec.security.m2m.externalIDP.basic.password",
		"spec.security.m2m.basic.password",
	},
}

// NewStores creates and initialises all stores. It panics if any store
// cannot be created (same semantics as rover-server).
func NewStores(ctx context.Context, cfg *rest.Config) *Stores {
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	s := &Stores{}

	s.APIExposureStore = NewOrDie[*apiv1.ApiExposure](ctx, dynamicClient, apiv1.GroupVersion.WithResource("apiexposures"), apiv1.GroupVersion.WithKind("ApiExposure"))
	s.APISubscriptionStore = NewOrDie[*apiv1.ApiSubscription](ctx, dynamicClient, apiv1.GroupVersion.WithResource("apisubscriptions"), apiv1.GroupVersion.WithKind("ApiSubscription"))
	s.ApplicationStore = NewOrDie[*applicationv1.Application](ctx, dynamicClient, applicationv1.GroupVersion.WithResource("applications"), applicationv1.GroupVersion.WithKind("Application"))
	s.ZoneStore = NewOrDie[*adminv1.Zone](ctx, dynamicClient, adminv1.GroupVersion.WithResource("zones"), adminv1.GroupVersion.WithKind("Zone"))
	s.ApprovalStore = NewOrDie[*approvalv1.Approval](ctx, dynamicClient, approvalv1.GroupVersion.WithResource("approvals"), approvalv1.GroupVersion.WithKind("Approval"))

	s.EventExposureStore = NewOrDie[*eventv1.EventExposure](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventexposures"), eventv1.GroupVersion.WithKind("EventExposure"))
	s.EventSubscriptionStore = NewOrDie[*eventv1.EventSubscription](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventsubscriptions"), eventv1.GroupVersion.WithKind("EventSubscription"))
	s.EventTypeStore = NewOrDie[*eventv1.EventType](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventtypes"), eventv1.GroupVersion.WithKind("EventType"))

	secretsAPI := secretsapi.NewSecrets()
	s.APIExposureSecretStore = secrets.WrapStore(s.APIExposureStore, SecretsForKinds["ApiExposure"], secrets.NewSecretManagerResolver(secretsAPI))
	s.APISubscriptionSecretStore = secrets.WrapStore(s.APISubscriptionStore, SecretsForKinds["ApiSubscription"], secrets.NewSecretManagerResolver(secretsAPI))

	return s
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
