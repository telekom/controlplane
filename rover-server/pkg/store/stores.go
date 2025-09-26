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
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
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
var ApiSubscriptionStore store.ObjectStore[*apiv1.ApiSubscription]
var ApiExposureStore store.ObjectStore[*apiv1.ApiExposure]
var ZoneStore store.ObjectStore[*adminv1.Zone]

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
	ApplicationStore = NewOrDie[*applicationv1.Application](ctx, applicationv1.GroupVersion.WithResource("applications"), applicationv1.GroupVersion.WithKind("Application"))
	ApiSubscriptionStore = NewOrDie[*apiv1.ApiSubscription](ctx, apiv1.GroupVersion.WithResource("apisubscriptions"), apiv1.GroupVersion.WithKind("ApiSubscription"))
	ApiExposureStore = NewOrDie[*apiv1.ApiExposure](ctx, apiv1.GroupVersion.WithResource("apiexposures"), apiv1.GroupVersion.WithKind("ApiExposure"))
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
			Filepath: viper.GetString("database.filepath"),
		},
	}

	return inmemory.NewSortableOrDie[T](ctx, storeOpts)
}
