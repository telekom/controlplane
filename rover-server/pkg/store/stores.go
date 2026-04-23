// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"

	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory"
	"github.com/telekom/controlplane/common-server/pkg/store/noop"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
)

// Stores is the dependency container holding all ObjectStore instances
// needed by the rover-server controllers and mappers.
type Stores struct {
	RoverStore       store.ObjectStore[*roverv1.Rover]
	RoverSecretStore store.ObjectStore[*roverv1.Rover]

	ApplicationStore       store.ObjectStore[*applicationv1.Application]
	ApplicationSecretStore store.ObjectStore[*applicationv1.Application]

	APISpecificationStore store.ObjectStore[*roverv1.ApiSpecification]
	APIStore              store.ObjectStore[*apiv1.Api]
	APISubscriptionStore  store.ObjectStore[*apiv1.ApiSubscription]
	APIExposureStore      store.ObjectStore[*apiv1.ApiExposure]

	EventSpecificationStore store.ObjectStore[*roverv1.EventSpecification]
	EventTypeStore          store.ObjectStore[*eventv1.EventType]
	EventExposureStore      store.ObjectStore[*eventv1.EventExposure]
	EventSubscriptionStore  store.ObjectStore[*eventv1.EventSubscription]
	ZoneStore               store.ObjectStore[*adminv1.Zone]
	EventConfigStore        store.ObjectStore[*eventv1.EventConfig]
}

var secretsForKinds = map[string][]string{
	"Rover": {
		"spec.clientSecret",
		"spec.subscriptions.#.api.security.m2m.client.clientSecret",
		"spec.subscriptions.#.api.security.m2m.basic.password",
		"spec.exposures.#.api.security.m2m.externalIDP.client.clientSecret",
		"spec.exposures.#.api.security.m2m.externalIDP.basic.password",
		"spec.exposures.#.api.security.m2m.basic.password",
	},
	"Application": {
		"status.clientSecret",
	},
}

// NewStores creates and initialises all stores. It panics if any store
// cannot be created (same semantics as the former InitOrDie).
func NewStores(ctx context.Context, cfg *rest.Config) *Stores {
	dynamicClient := dynamic.NewForConfigOrDie(cfg)

	s := &Stores{}

	s.RoverStore = NewOrDie[*roverv1.Rover](ctx, dynamicClient, roverv1.GroupVersion.WithResource("rovers"), roverv1.GroupVersion.WithKind("Rover"))
	s.APISpecificationStore = NewOrDie[*roverv1.ApiSpecification](ctx, dynamicClient, roverv1.GroupVersion.WithResource("apispecifications"), roverv1.GroupVersion.WithKind("ApiSpecification"))
	s.APIStore = NewOrDie[*apiv1.Api](ctx, dynamicClient, apiv1.GroupVersion.WithResource("apis"), apiv1.GroupVersion.WithKind("Api"))
	s.ApplicationStore = NewOrDie[*applicationv1.Application](ctx, dynamicClient, applicationv1.GroupVersion.WithResource("applications"), applicationv1.GroupVersion.WithKind("Application"))
	s.APISubscriptionStore = NewOrDie[*apiv1.ApiSubscription](ctx, dynamicClient, apiv1.GroupVersion.WithResource("apisubscriptions"), apiv1.GroupVersion.WithKind("ApiSubscription"))
	s.APIExposureStore = NewOrDie[*apiv1.ApiExposure](ctx, dynamicClient, apiv1.GroupVersion.WithResource("apiexposures"), apiv1.GroupVersion.WithKind("ApiExposure"))

	if cconfig.FeaturePubSub.IsEnabled() {
		s.EventSpecificationStore = NewOrDie[*roverv1.EventSpecification](ctx, dynamicClient, roverv1.GroupVersion.WithResource("eventspecifications"), roverv1.GroupVersion.WithKind("EventSpecification"))
		s.EventTypeStore = NewOrDie[*eventv1.EventType](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventtypes"), eventv1.GroupVersion.WithKind("EventType"))
		s.EventExposureStore = NewOrDie[*eventv1.EventExposure](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventexposures"), eventv1.GroupVersion.WithKind("EventExposure"))
		s.EventSubscriptionStore = NewOrDie[*eventv1.EventSubscription](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventsubscriptions"), eventv1.GroupVersion.WithKind("EventSubscription"))
		s.EventConfigStore = NewOrDie[*eventv1.EventConfig](ctx, dynamicClient, eventv1.GroupVersion.WithResource("eventconfigs"), eventv1.GroupVersion.WithKind("EventConfig"))
	} else {
		s.EventSpecificationStore = noop.NewStore[*roverv1.EventSpecification](roverv1.GroupVersion.WithResource("eventspecifications"), roverv1.GroupVersion.WithKind("EventSpecification"))
		s.EventTypeStore = noop.NewStore[*eventv1.EventType](eventv1.GroupVersion.WithResource("eventtypes"), eventv1.GroupVersion.WithKind("EventType"))
		s.EventExposureStore = noop.NewStore[*eventv1.EventExposure](eventv1.GroupVersion.WithResource("eventexposures"), eventv1.GroupVersion.WithKind("EventExposure"))
		s.EventSubscriptionStore = noop.NewStore[*eventv1.EventSubscription](eventv1.GroupVersion.WithResource("eventsubscriptions"), eventv1.GroupVersion.WithKind("EventSubscription"))
		s.EventConfigStore = noop.NewStore[*eventv1.EventConfig](eventv1.GroupVersion.WithResource("eventconfigs"), eventv1.GroupVersion.WithKind("EventConfig"))
	}

	s.ZoneStore = NewOrDie[*adminv1.Zone](ctx, dynamicClient, adminv1.GroupVersion.WithResource("zones"), adminv1.GroupVersion.WithKind("Zone"))

	secretsAPI := secretsapi.NewSecrets()
	s.RoverSecretStore = secrets.WrapStore(s.RoverStore, secretsForKinds["Rover"], secrets.NewSecretManagerResolver(secretsAPI))
	s.ApplicationSecretStore = secrets.WrapStore(s.ApplicationStore, secretsForKinds["Application"], secrets.NewSecretManagerResolver(secretsAPI))

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
