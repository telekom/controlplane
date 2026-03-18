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
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory"
	"github.com/telekom/controlplane/common-server/pkg/store/noop"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	secretsapi "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var RoverStore store.ObjectStore[*roverv1.Rover]
var RoverSecretStore store.ObjectStore[*roverv1.Rover]

var ApplicationStore store.ObjectStore[*applicationv1.Application]
var ApplicationSecretStore store.ObjectStore[*applicationv1.Application]

var ApiSpecificationStore store.ObjectStore[*roverv1.ApiSpecification]
var ApiStore store.ObjectStore[*apiv1.Api]
var ApiSubscriptionStore store.ObjectStore[*apiv1.ApiSubscription]
var ApiExposureStore store.ObjectStore[*apiv1.ApiExposure]

var EventSpecificationStore store.ObjectStore[*roverv1.EventSpecification]
var EventTypeStore store.ObjectStore[*eventv1.EventType]
var EventExposureStore store.ObjectStore[*eventv1.EventExposure]
var EventSubscriptionStore store.ObjectStore[*eventv1.EventSubscription]
var ZoneStore store.ObjectStore[*adminv1.Zone]
var EventConfigStore store.ObjectStore[*eventv1.EventConfig]

var dynamicClient dynamic.Interface

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

var InitOrDie = func(ctx context.Context, cfg *rest.Config) {
	dynamicClient = dynamic.NewForConfigOrDie(cfg)

	RoverStore = NewOrDie[*roverv1.Rover](ctx, roverv1.GroupVersion.WithResource("rovers"), roverv1.GroupVersion.WithKind("Rover"))
	ApiSpecificationStore = NewOrDie[*roverv1.ApiSpecification](ctx, roverv1.GroupVersion.WithResource("apispecifications"), roverv1.GroupVersion.WithKind("ApiSpecification"))
	ApiStore = NewOrDie[*apiv1.Api](ctx, apiv1.GroupVersion.WithResource("apis"), apiv1.GroupVersion.WithKind("Api"))
	ApplicationStore = NewOrDie[*applicationv1.Application](ctx, applicationv1.GroupVersion.WithResource("applications"), applicationv1.GroupVersion.WithKind("Application"))
	ApiSubscriptionStore = NewOrDie[*apiv1.ApiSubscription](ctx, apiv1.GroupVersion.WithResource("apisubscriptions"), apiv1.GroupVersion.WithKind("ApiSubscription"))
	ApiExposureStore = NewOrDie[*apiv1.ApiExposure](ctx, apiv1.GroupVersion.WithResource("apiexposures"), apiv1.GroupVersion.WithKind("ApiExposure"))

	if cconfig.FeaturePubSub.IsEnabled() {
		EventSpecificationStore = NewOrDie[*roverv1.EventSpecification](ctx, roverv1.GroupVersion.WithResource("eventspecifications"), roverv1.GroupVersion.WithKind("EventSpecification"))
		EventTypeStore = NewOrDie[*eventv1.EventType](ctx, eventv1.GroupVersion.WithResource("eventtypes"), eventv1.GroupVersion.WithKind("EventType"))
		EventExposureStore = NewOrDie[*eventv1.EventExposure](ctx, eventv1.GroupVersion.WithResource("eventexposures"), eventv1.GroupVersion.WithKind("EventExposure"))
		EventSubscriptionStore = NewOrDie[*eventv1.EventSubscription](ctx, eventv1.GroupVersion.WithResource("eventsubscriptions"), eventv1.GroupVersion.WithKind("EventSubscription"))
		EventConfigStore = NewOrDie[*eventv1.EventConfig](ctx, eventv1.GroupVersion.WithResource("eventconfigs"), eventv1.GroupVersion.WithKind("EventConfig"))
	} else {
		EventSpecificationStore = noop.NewStore[*roverv1.EventSpecification](roverv1.GroupVersion.WithResource("eventspecifications"), roverv1.GroupVersion.WithKind("EventSpecification"))
		EventTypeStore = noop.NewStore[*eventv1.EventType](eventv1.GroupVersion.WithResource("eventtypes"), eventv1.GroupVersion.WithKind("EventType"))
		EventExposureStore = noop.NewStore[*eventv1.EventExposure](eventv1.GroupVersion.WithResource("eventexposures"), eventv1.GroupVersion.WithKind("EventExposure"))
		EventSubscriptionStore = noop.NewStore[*eventv1.EventSubscription](eventv1.GroupVersion.WithResource("eventsubscriptions"), eventv1.GroupVersion.WithKind("EventSubscription"))
		EventConfigStore = noop.NewStore[*eventv1.EventConfig](eventv1.GroupVersion.WithResource("eventconfigs"), eventv1.GroupVersion.WithKind("EventConfig"))
	}

	ZoneStore = NewOrDie[*adminv1.Zone](ctx, adminv1.GroupVersion.WithResource("zones"), adminv1.GroupVersion.WithKind("Zone"))

	secretsApi := secretsapi.NewSecrets()
	RoverSecretStore = secrets.WrapStore(RoverStore, secretsForKinds["Rover"], secrets.NewSecretManagerResolver(secretsApi))
	ApplicationSecretStore = secrets.WrapStore(ApplicationStore, secretsForKinds["Application"], secrets.NewSecretManagerResolver(secretsApi))
}

func NewOrDie[T store.Object](ctx context.Context, gvr schema.GroupVersionResource, gvk schema.GroupVersionKind) store.ObjectStore[T] {
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
