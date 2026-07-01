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

	// --- Fetch Zone early to auto-resolve optional realm references ---

	myZone, err := util.GetZone(ctx, obj.Spec.Zone.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get zone for EventConfig's zone reference %q", obj.Spec.Zone.String())
	}

	if myZone.Status.Namespace != obj.Namespace {
		return ctrlerrors.BlockedErrorf("EventConfig must be located in the correlated zone-namespace %q", myZone.Status.Namespace)
	}

	// --- Resolve effective mesh config (nil means full mesh) ---

	meshCfg := obj.Spec.Mesh
	if meshCfg == nil {
		meshCfg = &eventv1.MeshConfig{FullMesh: true}
	}

	// --- Identity Clients ---

	adminClient, adminTokenUrl, err := h.resolveAndCreateAdminClient(ctx, obj, myZone)
	if err != nil {
		return err
	}
	obj.Status.AdminClient = eventv1.NewObservedObjectRef(adminClient)
	logger.V(1).Info("identity AdminClient created/updated", "client", adminClient.Name)

	meshClient, err := h.resolveAndCreateMeshClient(ctx, obj, myZone, meshCfg)
	if err != nil {
		return err
	}
	obj.Status.MeshClient = eventv1.NewObservedObjectRef(meshClient)
	logger.V(1).Info("identity MeshClient created/updated", "client", meshClient.Name)

	// --- EventStore ---

	eventStore, err := h.createEventStore(ctx, obj, adminClient, adminTokenUrl)
	if err != nil {
		return errors.Wrap(err, "failed to create EventStore")
	}
	obj.Status.EventStore = types.ObjectRefFromObject(eventStore)
	logger.V(1).Info("EventStore created/updated", "eventStore", eventStore.Name)

	// --- Routes ---

	if err = h.createCallbackRoutes(ctx, obj, myZone, meshCfg); err != nil {
		return errors.Wrap(err, "failed to create callback Routes")
	}
	logger.V(1).Info("Callback Routes created/updated", "count", len(obj.Status.ProxyCallbackRoutes))

	if obj.Spec.VoyagerApiUrl != "" {
		if err = h.createVoyagerRoutes(ctx, obj, myZone, meshCfg); err != nil {
			return errors.Wrap(err, "failed to create voyager Routes")
		}
		logger.V(1).Info("Voyager Routes created/updated", "count", len(obj.Status.ProxyVoyagerRoutes))
	}

	if err = h.createPublishRoute(ctx, obj, myZone); err != nil {
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

// resolveAndCreateAdminClient resolves the admin realm (from zone if not explicitly specified)
// and creates/updates the identity client for admin access. Returns the client and the token URL.
func (h *EventConfigHandler) resolveAndCreateAdminClient(ctx context.Context, obj *eventv1.EventConfig, zone *adminv1.Zone) (*identityv1.Client, string, error) {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	clientCfg := obj.Spec.Admin.Client
	if clientCfg.Realm.IsEmpty() {
		if zone.Status.InternalIdentityRealm == nil {
			return nil, "", ctrlerrors.BlockedErrorf("Zone %q does not have an internal identity realm yet", zone.Name)
		}
		clientCfg.Realm = *zone.Status.InternalIdentityRealm
		logger.V(1).Info("Auto-resolved admin client realm from zone", "realm", clientCfg.Realm.String())
	}

	realm := &identityv1.Realm{}
	if err := c.Get(ctx, clientCfg.Realm.K8s(), realm); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, "", ctrlerrors.BlockedErrorf("referenced identity Realm %q not found", clientCfg.Realm.String())
		}
		return nil, "", errors.Wrapf(err, "failed to get identity Realm %q", clientCfg.Realm.String())
	}

	tokenUrl := realm.Status.IssuerUrl + tokenUrlSuffix
	if tokenUrl == "" {
		return nil, "", ctrlerrors.BlockedErrorf("identity Realm %s has no issuerUrl yet", realm.Name)
	}

	identityClient, err := h.createIdentityClient(ctx, obj, &clientCfg)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to create admin identity Client")
	}

	return identityClient, tokenUrl, nil
}

// resolveAndCreateMeshClient resolves the mesh realm (from zone if not explicitly specified)
// and creates/updates the identity client for cross-zone mesh communication.
func (h *EventConfigHandler) resolveAndCreateMeshClient(ctx context.Context, obj *eventv1.EventConfig, zone *adminv1.Zone, meshCfg *eventv1.MeshConfig) (*identityv1.Client, error) {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	clientCfg := meshCfg.Client
	if clientCfg.Realm.IsEmpty() {
		if zone.Status.IdentityRealm == nil {
			return nil, ctrlerrors.BlockedErrorf("Zone %q does not have a default identity realm yet", zone.Name)
		}
		clientCfg.Realm = *zone.Status.IdentityRealm
		logger.V(1).Info("Auto-resolved mesh client realm from zone", "realm", clientCfg.Realm.String())
	}

	realm := &identityv1.Realm{}
	if err := c.Get(ctx, clientCfg.Realm.K8s(), realm); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return nil, ctrlerrors.BlockedErrorf("referenced identity Realm %q not found", clientCfg.Realm.String())
		}
		return nil, errors.Wrapf(err, "failed to get identity Realm %q", clientCfg.Realm.String())
	}

	identityClient, err := h.createIdentityClient(ctx, obj, &clientCfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create mesh identity Client")
	}

	return identityClient, nil
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

func (h *EventConfigHandler) createCallbackRoutes(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone, meshCfg *eventv1.MeshConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	realmName := adminv1.RealmNameFromContext(ctx)

	otherEventConfigs := &eventv1.EventConfigList{}
	err := c.List(ctx, otherEventConfigs)
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

	// Proxy routes use the source zone's LMS issuer (mesh-client authentication)
	var proxyTrustedIssuers []string
	if myZone.Status.Links.LmsIssuer != "" {
		proxyTrustedIssuers = []string{myZone.Status.Links.LmsIssuer}
	}

	logger.V(1).Info("Creating proxy callback Routes for other zones", "count", len(otherZones))
	routes, err := util.CreateCallbackProxyRoutes(ctx, meshCfg, myZone, otherZones,
		util.WithOwner(obj),
		util.WithTrustedIssuers(proxyTrustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create callback proxy Routes")
	}
	logger.V(1).Info("Created proxy callback Routes", "count", len(routes))
	obj.Status.ProxyCallbackRoutes = make([]types.ObjectRef, 0, len(routes))
	obj.Status.ProxyCallbackURLs = make(map[string]string, len(routes))

	for zoneName, route := range routes {
		obj.Status.ProxyCallbackRoutes = append(obj.Status.ProxyCallbackRoutes, *types.ObjectRefFromObject(route))
		obj.Status.ProxyCallbackURLs[zoneName] = util.RouteDownstreamURL(route)
	}

	// Primary callback route: trusted issuers = [IDP issuer] + [LMS issuers from proxy zones]
	isProxyTarget := len(obj.Status.ProxyCallbackRoutes) > 0
	primaryTrustedIssuers := collectPrimaryTrustedIssuers(myZone, otherZones, isProxyTarget)

	myCallbackRoute, err := util.CreateCallbackRoute(ctx, myZone,
		util.WithOwner(obj),
		util.WithProxyTarget(isProxyTarget),
		util.WithTrustedIssuers(primaryTrustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create callback Route for own zone")
	}
	obj.Status.CallbackRoute = types.ObjectRefFromObject(myCallbackRoute)
	obj.Status.CallbackURL = util.RouteDownstreamURL(myCallbackRoute)

	return nil
}

func (h *EventConfigHandler) createPublishRoute(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone) error {
	realmName := adminv1.RealmNameFromContext(ctx)

	// Publish routes are accessed by event publishers (external services) using IDP tokens
	var trustedIssuers []string
	if myZone.Status.Links.Issuer != "" {
		trustedIssuers = []string{myZone.Status.Links.Issuer}
	}

	route, err := util.CreatePublishRoute(ctx, myZone, obj,
		util.WithTrustedIssuers(trustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create publish Route")
	}
	obj.Status.PublishRoute = types.ObjectRefFromObject(route)
	obj.Status.PublishURL = util.RouteDownstreamURL(route)

	return nil
}

func (h *EventConfigHandler) createVoyagerRoutes(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone, meshCfg *eventv1.MeshConfig) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	realmName := adminv1.RealmNameFromContext(ctx)

	otherEventConfigs := &eventv1.EventConfigList{}
	err := c.List(ctx, otherEventConfigs)
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

	// Proxy routes use the source zone's LMS issuer (mesh-client authentication)
	var proxyTrustedIssuers []string
	if myZone.Status.Links.LmsIssuer != "" {
		proxyTrustedIssuers = []string{myZone.Status.Links.LmsIssuer}
	}

	logger.V(1).Info("Creating proxy voyager Routes for other zones", "count", len(otherZones))
	routes, err := util.CreateVoyagerProxyRoutes(ctx, meshCfg, myZone, otherZones,
		util.WithOwner(obj),
		util.WithTrustedIssuers(proxyTrustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create voyager proxy Routes")
	}
	logger.V(1).Info("Created proxy voyager Routes", "count", len(routes))
	obj.Status.ProxyVoyagerRoutes = make([]types.ObjectRef, 0, len(routes))
	obj.Status.ProxyVoyagerURLs = make(map[string]string, len(routes))

	for zoneName, route := range routes {
		obj.Status.ProxyVoyagerRoutes = append(obj.Status.ProxyVoyagerRoutes, *types.ObjectRefFromObject(route))
		obj.Status.ProxyVoyagerURLs[zoneName] = util.RouteDownstreamURL(route)
	}

	// Primary voyager route: trusted issuers = [IDP issuer] + [LMS issuers from proxy zones]
	isProxyTarget := len(obj.Status.ProxyVoyagerRoutes) > 0
	primaryTrustedIssuers := collectPrimaryTrustedIssuers(myZone, otherZones, isProxyTarget)

	myVoyagerRoute, err := util.CreateVoyagerRoute(ctx, myZone, obj,
		util.WithOwner(obj),
		util.WithProxyTarget(isProxyTarget),
		util.WithTrustedIssuers(primaryTrustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create voyager Route for own zone")
	}
	obj.Status.VoyagerRoute = types.ObjectRefFromObject(myVoyagerRoute)
	obj.Status.VoyagerURL = util.RouteDownstreamURL(myVoyagerRoute)

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

// collectPrimaryTrustedIssuers builds the list of trusted token issuers for a primary event route.
// It includes the zone's own IDP issuer (for consumer access) and the LMS issuers from
// all cross-zone proxy zones (for mesh-client access from proxy routes).
func collectPrimaryTrustedIssuers(myZone *adminv1.Zone, otherZones []*adminv1.Zone, isProxyTarget bool) []string {
	var issuers []string

	// Zone's IDP issuer: all event routes are accessed by external services
	if myZone.Status.Links.Issuer != "" {
		issuers = append(issuers, myZone.Status.Links.Issuer)
	}

	// LMS issuers from proxy zones: when cross-zone proxies forward traffic
	// to this primary route, they present LMS tokens from their respective zones
	if isProxyTarget {
		for _, otherZone := range otherZones {
			if otherZone.Status.Links.LmsIssuer != "" {
				issuers = append(issuers, otherZone.Status.Links.LmsIssuer)
			}
		}
	}

	return issuers
}
