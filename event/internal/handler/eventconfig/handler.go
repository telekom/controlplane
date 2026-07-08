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

	// Proxy zones have no admin client of their own; their EventStore authenticates
	// to the target zone's configuration backend using the target's admin client.
	var adminClient *identityv1.Client
	var adminTokenUrl string
	if obj.IsProxy() {
		obj.Status.AdminClient = nil
	} else {
		adminClient, adminTokenUrl, err = h.resolveAndCreateAdminClient(ctx, obj, myZone)
		if err != nil {
			return err
		}
		obj.Status.AdminClient = eventv1.NewObservedObjectRef(adminClient)
		logger.V(1).Info("identity AdminClient created/updated", "client", adminClient.Name)
	}

	meshClient, err := h.resolveAndCreateMeshClient(ctx, obj, myZone, meshCfg)
	if err != nil {
		return err
	}
	obj.Status.MeshClient = eventv1.NewObservedObjectRef(meshClient)
	logger.V(1).Info("identity MeshClient created/updated", "client", meshClient.Name)

	// --- EventStore ---

	var eventStore *pubsubv1.EventStore
	if obj.IsProxy() {
		eventStore, err = h.createProxyEventStore(ctx, obj)
	} else {
		eventStore, err = h.createEventStore(ctx, obj, adminClient, adminTokenUrl)
	}
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

	// Voyager Routes: local zones expose their own backend; proxy zones forward their
	// own-zone Route to the target and still participate in the full mesh.
	if obj.IsProxy() {
		if err = h.createProxyVoyagerRoutes(ctx, obj, myZone, meshCfg); err != nil {
			return errors.Wrap(err, "failed to create proxy voyager Routes")
		}
		logger.V(1).Info("Proxy voyager Routes created/updated", "count", len(obj.Status.ProxyVoyagerRoutes))
	} else if obj.Spec.Local.VoyagerApiUrl != "" {
		if err = h.createVoyagerRoutes(ctx, obj, myZone, meshCfg); err != nil {
			return errors.Wrap(err, "failed to create voyager Routes")
		}
		logger.V(1).Info("Voyager Routes created/updated", "count", len(obj.Status.ProxyVoyagerRoutes))
	}

	if obj.IsProxy() {
		if err = h.createProxyPublishRoute(ctx, obj, myZone); err != nil {
			return errors.Wrap(err, "failed to create proxy publish Route")
		}
		logger.V(1).Info("Proxy publish Route created/updated")
	} else {
		if err = h.createPublishRoute(ctx, obj, myZone); err != nil {
			return errors.Wrap(err, "failed to create publish Route")
		}
		logger.V(1).Info("Publish Route created/updated")
	}

	// --- Finalize status conditions ---

	if !c.AllReady() {
		obj.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		obj.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady, "Waiting for child resources"))
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

	obj.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "EventConfig has been provisioned"))
	obj.SetCondition(condition.NewDoneProcessingCondition("EventConfig has been provisioned"))

	return nil
}

func (h *EventConfigHandler) Delete(ctx context.Context, obj *eventv1.EventConfig) error {
	// Child resources are cleaned up by the janitor client (ownership tracking).
	// No additional manual cleanup needed.
	return nil
}

// resolveAndCreateAdminClient resolves the admin realm (from zone if not explicitly specified)
// and creates/updates the identity client for admin access. Returns the client and the token URL.
func (h *EventConfigHandler) resolveAndCreateAdminClient(ctx context.Context, obj *eventv1.EventConfig, zone *adminv1.Zone) (*identityv1.Client, string, error) {
	logger := log.FromContext(ctx)
	c := cclient.ClientFromContextOrDie(ctx)

	clientCfg := obj.Spec.Local.Admin.Client
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

// createIdentityClient is a utility function to create or update an identityv1.Client resource for a given EventConfig and ClientConfig.
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

// listMeshPeerZones lists the zones of all other EventConfigs (excluding obj's own zone).
// realPeers: non-proxy peers that host a backend (a proxy Route can point at them).
// allPeers:  every peer (real + proxy); proxy peers are trust-only, no Route target.
func (h *EventConfigHandler) listMeshPeerZones(ctx context.Context, obj *eventv1.EventConfig) (realPeerZones, allPeerZones []*adminv1.Zone, err error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	otherEventConfigs := &eventv1.EventConfigList{}
	if err = c.List(ctx, otherEventConfigs); err != nil {
		return nil, nil, errors.Wrap(err, "failed to list other EventConfigs")
	}
	logger.V(1).Info("Fetched other EventConfigs", "count", len(otherEventConfigs.Items))

	for i := range otherEventConfigs.Items {
		other := &otherEventConfigs.Items[i]
		if types.Equals(other, obj) {
			continue
		}
		otherZone, zoneErr := util.GetZone(ctx, other.Spec.Zone.K8s())
		if zoneErr != nil {
			return nil, nil, errors.Wrapf(zoneErr, "failed to get zone for other EventConfig %q", other.Name)
		}
		allPeerZones = append(allPeerZones, otherZone)
		if !other.IsProxy() {
			realPeerZones = append(realPeerZones, otherZone)
		}
	}
	return realPeerZones, allPeerZones, nil
}

func (h *EventConfigHandler) createCallbackRoutes(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone, meshCfg *eventv1.MeshConfig) error {
	logger := log.FromContext(ctx)

	realmName := myZone.Status.RealmName

	// Callbacks proxy to every peer (proxy zones also expose a local callback primary),
	// so route targets and trust both use the full peer set.
	_, otherZones, err := h.listMeshPeerZones(ctx, obj)
	if err != nil {
		return err
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
	realmName := myZone.Status.RealmName

	// Publish routes are accessed by event publishers (external services) using IDP tokens
	var trustedIssuers []string
	if myZone.Status.Links.Issuer != "" {
		trustedIssuers = []string{myZone.Status.Links.Issuer}
	}

	// Proxy zones targeting this zone forward publish traffic authenticated with an
	// LMS (mesh) token issued in their own zone. Trust those issuers so the target
	// gateway accepts the proxied publish requests.
	proxySourceZones, err := h.findProxySourceZones(ctx, obj, myZone)
	if err != nil {
		return err
	}
	for _, pz := range proxySourceZones {
		if pz.Status.Links.LmsIssuer != "" {
			trustedIssuers = append(trustedIssuers, pz.Status.Links.LmsIssuer)
		}
	}

	route, err := util.CreatePublishRoute(ctx, myZone, obj,
		util.WithOwner(obj),
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

// findProxySourceZones returns the zones of all proxy EventConfigs whose target is myZone.
// These are the zones that forward event traffic into this (local) zone via mesh-client tokens.
func (h *EventConfigHandler) findProxySourceZones(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone) ([]*adminv1.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventConfigs := &eventv1.EventConfigList{}
	if err := c.List(ctx, eventConfigs); err != nil {
		return nil, errors.Wrap(err, "failed to list EventConfigs")
	}

	var sourceZones []*adminv1.Zone
	for i := range eventConfigs.Items {
		other := &eventConfigs.Items[i]
		if types.Equals(other, obj) || !other.IsProxy() {
			continue
		}
		if other.Spec.Proxy.TargetZone.Name != myZone.Name {
			continue
		}
		sourceZone, err := util.GetZone(ctx, other.Spec.Zone.K8s())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get zone for proxy EventConfig %q", other.Name)
		}
		sourceZones = append(sourceZones, sourceZone)
	}

	return sourceZones, nil
}

func (h *EventConfigHandler) createVoyagerRoutes(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone, meshCfg *eventv1.MeshConfig) error {
	logger := log.FromContext(ctx)

	realmName := myZone.Status.RealmName

	// realPeerZones excludes proxy peers (they run no local Voyager backend, so there
	// is no primary Route to point a proxy Route at). allPeerZones includes proxy peers,
	// which must be trusted on the primary Route because they read this zone's Voyager.
	realPeerZones, allPeerZones, err := h.listMeshPeerZones(ctx, obj)
	if err != nil {
		return err
	}

	// Proxy routes use the source zone's LMS issuer (mesh-client authentication)
	var proxyTrustedIssuers []string
	if myZone.Status.Links.LmsIssuer != "" {
		proxyTrustedIssuers = []string{myZone.Status.Links.LmsIssuer}
	}

	logger.V(1).Info("Creating proxy voyager Routes for other zones", "count", len(realPeerZones))
	routes, err := util.CreateVoyagerProxyRoutes(ctx, meshCfg, myZone, realPeerZones,
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

	// Primary voyager route trust includes this zone's IDP issuer plus the LMS issuers of
	// all peers (proxy and non-proxy) that proxy reads to this primary. isProxyTarget is
	// true whenever any peer exists, since every mesh peer forwards to this primary Route.
	isProxyTarget := len(allPeerZones) > 0
	primaryTrustedIssuers := collectPrimaryTrustedIssuers(myZone, allPeerZones, isProxyTarget)

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

// createProxyVoyagerRoutes builds Voyager Routes for a proxy zone. The proxy zone runs no
// Voyager backend, so its own-zone Route forwards to the target zone's gateway. It also
// participates in the full mesh: it builds proxy Routes to every other meshed non-proxy
// zone, exactly like a local zone. If the target zone exposes no Voyager backend, no
// Voyager Routes are created.
func (h *EventConfigHandler) createProxyVoyagerRoutes(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone, meshCfg *eventv1.MeshConfig) error {
	logger := log.FromContext(ctx)

	realmName := myZone.Status.RealmName
	targetZoneName := obj.Spec.Proxy.TargetZone.Name

	targetCfg, err := util.GetEventConfigForZone(ctx, targetZoneName)
	if err != nil {
		return err // BlockedError propagates so the proxy requeues until the target is ready
	}
	if targetCfg.IsProxy() || targetCfg.Spec.Local == nil {
		return ctrlerrors.BlockedErrorf("target zone %q of proxy EventConfig %q must be a local (non-proxy) zone", targetZoneName, obj.Name)
	}
	if targetCfg.Spec.Local.VoyagerApiUrl == "" {
		logger.V(0).Info("Target zone exposes no Voyager backend; skipping Voyager Routes for proxy zone", "targetZone", targetZoneName)
		return nil
	}

	targetZone, err := util.GetZone(ctx, targetCfg.Spec.Zone.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get target zone %q", targetZoneName)
	}

	// Own-zone Route: serves /horizon/voyager/v1 + /horizon-{myZone}/voyager/v1, forwarding to the target
	// zone's gateway. Readers in this zone authenticate with IDP tokens (local trust).
	var ownTrustedIssuers []string
	if myZone.Status.Links.Issuer != "" {
		ownTrustedIssuers = []string{myZone.Status.Links.Issuer}
	}
	ownRoute, err := util.CreateProxyLocalVoyagerRoute(ctx, myZone, targetZone,
		util.WithOwner(obj),
		util.WithTrustedIssuers(ownTrustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create own proxy voyager Route")
	}
	obj.Status.VoyagerRoute = types.ObjectRefFromObject(ownRoute)
	obj.Status.VoyagerURL = util.RouteDownstreamURL(ownRoute)

	// Mesh Routes: proxy to every other meshed non-proxy zone, like a local zone.
	// Proxy routes authenticate to the target primary with this zone's LMS (mesh) issuer.
	realPeerZones, _, err := h.listMeshPeerZones(ctx, obj)
	if err != nil {
		return err
	}

	var proxyTrustedIssuers []string
	if myZone.Status.Links.LmsIssuer != "" {
		proxyTrustedIssuers = []string{myZone.Status.Links.LmsIssuer}
	}

	logger.V(1).Info("Creating proxy voyager Routes for other zones", "count", len(realPeerZones))
	routes, err := util.CreateVoyagerProxyRoutes(ctx, meshCfg, myZone, realPeerZones,
		util.WithOwner(obj),
		util.WithTrustedIssuers(proxyTrustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create voyager proxy Routes")
	}
	obj.Status.ProxyVoyagerRoutes = make([]types.ObjectRef, 0, len(routes))
	obj.Status.ProxyVoyagerURLs = make(map[string]string, len(routes))
	for zoneName, route := range routes {
		obj.Status.ProxyVoyagerRoutes = append(obj.Status.ProxyVoyagerRoutes, *types.ObjectRefFromObject(route))
		obj.Status.ProxyVoyagerURLs[zoneName] = util.RouteDownstreamURL(route)
	}

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
			Url:          obj.Spec.Local.Admin.Url,
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

// createProxyEventStore creates a pubsub.EventStore for a proxy zone. Instead of a local
// admin client, it authenticates to the target zone's configuration backend using the
// target zone's admin client credentials and admin realm token endpoint.
func (h *EventConfigHandler) createProxyEventStore(ctx context.Context, obj *eventv1.EventConfig) (*pubsubv1.EventStore, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	targetZoneName := obj.Spec.Proxy.TargetZone.Name

	targetCfg, err := util.GetEventConfigForZone(ctx, targetZoneName)
	if err != nil {
		return nil, err // BlockedError propagates so the proxy requeues until the target is ready
	}
	if targetCfg.IsProxy() || targetCfg.Spec.Local == nil {
		return nil, ctrlerrors.BlockedErrorf("target zone %q of proxy EventConfig %q must be a local (non-proxy) zone", targetZoneName, obj.Name)
	}

	targetZone, err := util.GetZone(ctx, targetCfg.Spec.Zone.K8s())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get target zone %q", targetZoneName)
	}

	adminCfg := targetCfg.Spec.Local.Admin
	tokenUrl, err := h.resolveAdminTokenUrl(ctx, &adminCfg.Client, targetZone)
	if err != nil {
		return nil, err
	}

	eventStore := &pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	}

	mutator := func() error {
		if refErr := controllerutil.SetControllerReference(obj, eventStore, c.Scheme()); refErr != nil {
			return errors.Wrap(refErr, "failed to set controller reference")
		}

		eventStore.Spec = pubsubv1.EventStoreSpec{
			Url:          adminCfg.Url,
			TokenUrl:     tokenUrl,
			ClientId:     adminCfg.Client.ClientId,
			ClientSecret: adminCfg.Client.ClientSecret,
		}
		return nil
	}

	if _, err := c.CreateOrUpdate(ctx, eventStore, mutator); err != nil {
		return nil, errors.Wrapf(err, "failed to create or update proxy EventStore %s", eventStore.Name)
	}

	return eventStore, nil
}

// resolveAdminTokenUrl resolves the OAuth2 token endpoint for an admin client config,
// falling back to the zone's internal identity realm when no realm is explicitly set.
func (h *EventConfigHandler) resolveAdminTokenUrl(ctx context.Context, clientCfg *eventv1.ClientConfig, zone *adminv1.Zone) (string, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	realmRef := clientCfg.Realm
	if realmRef.IsEmpty() {
		if zone.Status.InternalIdentityRealm == nil {
			return "", ctrlerrors.BlockedErrorf("Zone %q does not have an internal identity realm yet", zone.Name)
		}
		realmRef = *zone.Status.InternalIdentityRealm
	}

	realm := &identityv1.Realm{}
	if err := c.Get(ctx, realmRef.K8s(), realm); err != nil {
		if apierrors.IsNotFound(errors.Cause(err)) {
			return "", ctrlerrors.BlockedErrorf("referenced identity Realm %q not found", realmRef.String())
		}
		return "", errors.Wrapf(err, "failed to get identity Realm %q", realmRef.String())
	}
	if realm.Status.IssuerUrl == "" {
		return "", ctrlerrors.BlockedErrorf("identity Realm %s has no issuerUrl yet", realm.Name)
	}

	return realm.Status.IssuerUrl + tokenUrlSuffix, nil
}

// createProxyPublishRoute creates the publish Route for a proxy zone as a proxy Route
// pointing at the target zone's gateway. Local publishers authenticate with the zone's
// IDP tokens on the downstream side; the gateway re-authenticates to the target with the
// mesh client.
func (h *EventConfigHandler) createProxyPublishRoute(ctx context.Context, obj *eventv1.EventConfig, myZone *adminv1.Zone) error {
	targetZone, err := util.GetZone(ctx, obj.Spec.Proxy.TargetZone.K8s())
	if err != nil {
		return errors.Wrapf(err, "failed to get target zone %q", obj.Spec.Proxy.TargetZone.String())
	}

	realmName := myZone.Status.RealmName

	// Publishers access the proxy publish route with IDP tokens, same as a primary route.
	var trustedIssuers []string
	if myZone.Status.Links.Issuer != "" {
		trustedIssuers = []string{myZone.Status.Links.Issuer}
	}

	route, err := util.CreatePublishProxyRoute(ctx, myZone, targetZone,
		util.WithOwner(obj),
		util.WithTrustedIssuers(trustedIssuers),
		util.WithRealmName(realmName),
	)
	if err != nil {
		return errors.Wrap(err, "failed to create proxy publish Route")
	}
	obj.Status.PublishRoute = types.ObjectRefFromObject(route)
	obj.Status.PublishURL = util.RouteDownstreamURL(route)

	return nil
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
