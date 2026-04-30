// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/handler"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

const tokenUrlSuffix = "/protocol/openid-connect/token"

var _ handler.Handler[*eventv1.EventConfig] = &EventConfigHandler{}

type EventConfigHandler struct{}

func (h *EventConfigHandler) CreateOrUpdate(ctx context.Context, obj *eventv1.EventConfig) error {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	// --- Admin Identity Client ---

	adminRealm := &identityv1.Realm{}
	err := c.Get(ctx, obj.Spec.Admin.Client.Realm.K8s(), adminRealm)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return ctrlerrors.BlockedErrorf("referenced identity Realm %q not found", obj.Spec.Admin.Client.Realm.String())
		}
		return errors.Wrapf(err, "failed to get identity Realm %q", obj.Spec.Admin.Client.Realm.String())
	}

	// Derive the token URL from the realm's issuer URL
	// This is used to configure the EventStore with the correct token endpoint for obtaining access tokens.
	adminClientTokenUrl := adminRealm.Status.IssuerUrl + tokenUrlSuffix
	if adminClientTokenUrl == "" {
		return ctrlerrors.BlockedErrorf("identity Realm %s has no issuerUrl yet", adminRealm.Name)
	}

	adminClient, err := h.createIdentityClient(ctx, obj, &obj.Spec.Admin.Client)
	if err != nil {
		return errors.Wrap(err, "failed to create identity Client")
	}
	obj.Status.AdminClient = eventv1.NewObservedObjectRef(adminClient)
	logger.V(1).Info("identity AdminClient created/updated", "client", adminClient.Name)

	// --- Mesh Identity Client ---

	meshRealm := &identityv1.Realm{}
	err = c.Get(ctx, obj.Spec.Mesh.Client.Realm.K8s(), meshRealm)
	if err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return ctrlerrors.BlockedErrorf("referenced identity Realm %q not found", obj.Spec.Mesh.Client.Realm.String())
		}
		return errors.Wrapf(err, "failed to get identity Realm %q", obj.Spec.Mesh.Client.Realm.String())
	}

	meshClient, err := h.createIdentityClient(ctx, obj, &obj.Spec.Mesh.Client)
	if err != nil {
		return errors.Wrap(err, "failed to create identity Client")
	}
	obj.Status.MeshClient = eventv1.NewObservedObjectRef(meshClient)
	logger.V(1).Info("identity MeshClient created/updated", "client", meshClient.Name)

	// --- EventStore ---

	eventStore, err := h.createEventStore(ctx, obj, adminClient, adminClientTokenUrl)
	if err != nil {
		return errors.Wrap(err, "failed to create EventStore")
	}
	obj.Status.EventStore = types.ObjectRefFromObject(eventStore)
	logger.V(1).Info("EventStore created/updated", "eventStore", eventStore.Name)

	// --- Routes ---

	if err = h.createCallbackRoutes(ctx, obj); err != nil {
		return errors.Wrap(err, "failed to create callback Routes")
	}
	logger.V(1).Info("Callback Routes created/updated", "count", len(obj.Status.ProxyCallbackRoutes))

	if obj.Spec.VoyagerApiUrl != "" {
		if err = h.createVoyagerRoutes(ctx, obj); err != nil {
			return errors.Wrap(err, "failed to create voyager Routes")
		}
		logger.V(1).Info("Voyager Routes created/updated", "count", len(obj.Status.ProxyVoyagerRoutes))
	}

	if err = h.createPublishRoute(ctx, obj); err != nil {
		return errors.Wrap(err, "failed to create publish Route")
	}
	logger.V(1).Info("Publish Route created/updated")

	// --- Finalize status conditions ---

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition("ChildResourcesNotReady",
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition("ChildResourcesNotReady", "Waiting for child resources"))
		return nil
	}

	// --- Cleanup old child resources that are no longer referenced

	deleted, err := c.CleanupAll(ctx, cclient.OwnedBy(obj))
	if err != nil {
		return errors.Wrap(err, "failed to cleanup old child resources")
	}
	if deleted > 0 {
		logger.V(1).Info("Cleaned up old child resources", "count", deleted)
	}

	// --- Done ---

	obj.SetCondition(condition.NewReadyCondition("EventConfigProvisioned", "EventConfig has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("EventConfig has been provisioned"))

	return nil
}

func (h *EventConfigHandler) Delete(ctx context.Context, obj *eventv1.EventConfig) error {
	// Child resources are cleaned up by the janitor client (ownership tracking).
	// No additional manual cleanup needed.
	return nil
}

// createIdentityClient creates an identity.Client for the event operator to authenticate with configuration backend.
func (h *EventConfigHandler) createIdentityClient(ctx context.Context, obj *eventv1.EventConfig, clientCfg *eventv1.ClientConfig) (*identityv1.Client, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	identityClient := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientCfg.ClientId,
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, identityClient, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		identityClient.Labels = map[string]string{
			config.DomainLabelKey: "event",
		}

		identityClient.Spec = identityv1.ClientSpec{
			Realm:        &clientCfg.Realm,
			ClientId:     clientCfg.ClientId,
			ClientSecret: clientCfg.ClientSecret,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, identityClient, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update identity Client %s", clientCfg.ClientId)
	}

	return identityClient, nil
}

func (h *EventConfigHandler) createCallbackRoutes(ctx context.Context, obj *eventv1.EventConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	myZone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get zone for EventConfig's zone reference %q", obj.Spec.Zone.String())
	}

	if myZone.Status.Namespace != obj.Namespace {
		return ctrlerrors.BlockedErrorf("EventConfig must be located in the correlated zone-namespace %q", myZone.Status.Namespace)
	}

	otherEventConfigs := &eventv1.EventConfigList{}
	err = c.List(ctx, otherEventConfigs)
	if err != nil {
		return errors.Wrap(err, "failed to list other EventConfigs")
	}
	logger.V(1).Info("Fetched other EventConfigs", "count", len(otherEventConfigs.Items))
	otherZones := make([]*adminv1.Zone, 0, len(otherEventConfigs.Items))

	for i := range otherEventConfigs.Items {
		other := &otherEventConfigs.Items[i]
		if types.Equals(other, obj) {
			continue
		}
		otherZone, zoneErr := util.GetZone(ctx, other.Spec.Zone.K8s())
		if zoneErr != nil {
			return errors.Wrapf(zoneErr, "failed to get zone for other EventConfig %q", other.Name)
		}
		otherZones = append(otherZones, otherZone)
	}

	logger.V(1).Info("Creating proxy callback Routes for other zones", "count", len(otherZones))
	routes, err := util.CreateCallbackProxyRoutes(ctx, &obj.Spec.Mesh, myZone, otherZones, util.WithOwner(obj))
	if err != nil {
		return errors.Wrap(err, "failed to create callback proxy Routes")
	}
	logger.V(1).Info("Created proxy callback Routes", "count", len(routes))
	obj.Status.ProxyCallbackRoutes = make([]types.ObjectRef, 0, len(routes))
	obj.Status.ProxyCallbackURLs = make(map[string]string, len(routes))

	for zoneName, route := range routes {
		obj.Status.ProxyCallbackRoutes = append(obj.Status.ProxyCallbackRoutes, *types.ObjectRefFromObject(route))
		obj.Status.ProxyCallbackURLs[zoneName] = route.Spec.Downstreams[0].Url()
	}

	isProxyTarget := len(obj.Status.ProxyCallbackRoutes) > 0
	myCallbackRoute, err := util.CreateCallbackRoute(ctx, myZone, util.WithOwner(obj), util.WithProxyTarget(isProxyTarget))
	if err != nil {
		return errors.Wrap(err, "failed to create callback Route for own zone")
	}
	obj.Status.CallbackRoute = types.ObjectRefFromObject(myCallbackRoute)
	obj.Status.CallbackURL = myCallbackRoute.Spec.Downstreams[0].Url()

	return nil
}

func (h *EventConfigHandler) createPublishRoute(ctx context.Context, obj *eventv1.EventConfig) error {
	myZone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get zone for EventConfig's zone reference %q", obj.Spec.Zone.String())
	}

	route, err := util.CreatePublishRoute(ctx, myZone, obj)
	if err != nil {
		return errors.Wrap(err, "failed to create publish Route")
	}
	obj.Status.PublishRoute = types.ObjectRefFromObject(route)
	obj.Status.PublishURL = route.Spec.Downstreams[0].Url()

	return nil
}

func (h *EventConfigHandler) createVoyagerRoutes(ctx context.Context, obj *eventv1.EventConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	myZone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get zone for EventConfig's zone reference %q", obj.Spec.Zone.String())
	}

	otherEventConfigs := &eventv1.EventConfigList{}
	err = c.List(ctx, otherEventConfigs)
	if err != nil {
		return errors.Wrap(err, "failed to list other EventConfigs")
	}
	logger.V(1).Info("Fetched other EventConfigs for voyager Routes", "count", len(otherEventConfigs.Items))
	otherZones := make([]*adminv1.Zone, 0, len(otherEventConfigs.Items))

	for i := range otherEventConfigs.Items {
		other := &otherEventConfigs.Items[i]
		if types.Equals(other, obj) {
			continue
		}
		otherZone, zoneErr := util.GetZone(ctx, other.Spec.Zone.K8s())
		if zoneErr != nil {
			return errors.Wrapf(zoneErr, "failed to get zone for other EventConfig %q", other.Name)
		}
		otherZones = append(otherZones, otherZone)
	}

	logger.V(1).Info("Creating proxy voyager Routes for other zones", "count", len(otherZones))
	routes, err := util.CreateVoyagerProxyRoutes(ctx, &obj.Spec.Mesh, myZone, otherZones, util.WithOwner(obj))
	if err != nil {
		return errors.Wrap(err, "failed to create voyager proxy Routes")
	}
	logger.V(1).Info("Created proxy voyager Routes", "count", len(routes))
	obj.Status.ProxyVoyagerRoutes = make([]types.ObjectRef, 0, len(routes))
	obj.Status.ProxyVoyagerURLs = make(map[string]string, len(routes))

	for zoneName, route := range routes {
		obj.Status.ProxyVoyagerRoutes = append(obj.Status.ProxyVoyagerRoutes, *types.ObjectRefFromObject(route))
		obj.Status.ProxyVoyagerURLs[zoneName] = route.Spec.Downstreams[0].Url()
	}

	isProxyTarget := len(obj.Status.ProxyVoyagerRoutes) > 0
	myVoyagerRoute, err := util.CreateVoyagerRoute(ctx, myZone, obj, util.WithOwner(obj), util.WithProxyTarget(isProxyTarget))
	if err != nil {
		return errors.Wrap(err, "failed to create voyager Route for own zone")
	}
	obj.Status.VoyagerRoute = types.ObjectRefFromObject(myVoyagerRoute)
	obj.Status.VoyagerURL = myVoyagerRoute.Spec.Downstreams[0].Url()

	return nil
}

// createEventStore creates a pubsub.EventStore with resolved configuration backend connection details.
func (h *EventConfigHandler) createEventStore(ctx context.Context, obj *eventv1.EventConfig, identityClient *identityv1.Client, tokenUrl string) (*pubsubv1.EventStore, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventStoreName := obj.Name
	eventStore := &pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventStoreName,
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		if err := controllerutil.SetControllerReference(obj, eventStore, c.Scheme()); err != nil {
			return errors.Wrap(err, "failed to set controller reference")
		}

		eventStore.Spec = pubsubv1.EventStoreSpec{
			Url:          obj.Spec.Admin.Url,
			TokenUrl:     tokenUrl,
			ClientId:     identityClient.Spec.ClientId,
			ClientSecret: identityClient.Spec.ClientSecret,
		}
		return nil
	}

	_, err := c.CreateOrUpdate(ctx, eventStore, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update EventStore %s", eventStoreName)
	}

	return eventStore, nil
}
