// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

type ListenerHandler struct{}

func (h *ListenerHandler) CreateOrUpdate(ctx context.Context, listener *spectrev1.Listener) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Step 1: Resolve consumer and provider Applications.
	consumerApp, err := h.resolveApplication(ctx, &listener.Spec.Consumer)
	if err != nil {
		return errors.Wrap(err, "failed to resolve consumer Application")
	}
	providerApp, err := h.resolveApplication(ctx, &listener.Spec.Provider)
	if err != nil {
		return errors.Wrap(err, "failed to resolve provider Application")
	}

	consumerId := consumerApp.Status.ClientId
	providerId := providerApp.Status.ClientId

	// Step 2: Resolve the owning SpectreApplication to get the appId.
	spectreApp, err := h.resolveSpectreApplication(ctx, listener)
	if err != nil {
		return errors.Wrap(err, "failed to resolve SpectreApplication")
	}
	appId := spectreApp.Status.Id

	// Step 3: Resolve zones.
	consumerZone, err := h.resolveZone(ctx, consumerApp)
	if err != nil {
		return errors.Wrap(err, "failed to resolve consumer zone")
	}
	providerZone, err := h.resolveZone(ctx, providerApp)
	if err != nil {
		return errors.Wrap(err, "failed to resolve provider zone")
	}

	listeningZone, err := util.GetListeningZone(ctx, providerZone, consumerZone)
	if err != nil {
		return errors.Wrap(err, "failed to determine listening zone")
	}

	// Step 4: Get EventConfig for zone.
	eventConfig, err := util.GetEventConfig(ctx, listeningZone)
	if err != nil {
		return errors.Wrap(err, "failed to get EventConfig")
	}

	if eventConfig.Status.CallbackURL == "" {
		return ctrlerrors.BlockedErrorf("EventConfig %q has no CallbackURL in status", eventConfig.Name)
	}

	// Step 5: Find EventStore in zone namespace.
	eventStore, err := h.findEventStore(ctx, listeningZone.Status.Namespace)
	if err != nil {
		return err
	}

	// Reject event-only Listeners early — creating ApprovalRequests for a
	// Listener that can never provision downstream resources wastes effort and
	// leaves orphaned CRs.
	if listener.Spec.ApiListener == nil {
		return ctrlerrors.BlockedErrorf("Listener %q has no ApiListener configured (event-only listeners are not yet supported)", listener.Name)
	}

	// Step 6: Create the provider approval (gate).
	listenerTeam := consumerApp.Spec.Team
	listenerEmail := consumerApp.Spec.TeamEmail

	approval, err := h.ensureApprovals(ctx, listener,
		listenerTeam, listenerEmail,
		providerApp.Spec.Team, providerApp.Spec.TeamEmail)
	if err != nil {
		return errors.Wrap(err, "failed to ensure approvals")
	}

	listener.Status.ProviderApproval = approval.providerApproval

	// Step 7: Gate — if NOT both granted, return early.
	if !approval.granted {
		return nil
	}

	logger.Info("Both approvals granted, provisioning downstream resources")

	// Step 8: Ensure shared generic Publisher.
	publisher, err := h.ensureGenericPublisher(ctx, eventStore)
	if err != nil {
		return errors.Wrap(err, "failed to ensure generic Publisher")
	}
	logger.Info("Ensured generic Publisher", "publisher", publisher.Name)

	// Step 9: Create RouteListener.
	apiBasePath := listener.Spec.ApiListener.ApiBasePath

	routeListener, err := h.ensureRouteListener(ctx, listener, listeningZone, appId, consumerId, providerId, apiBasePath)
	if err != nil {
		return errors.Wrap(err, "failed to ensure RouteListener")
	}
	listener.Status.RouteListener = ctypes.ObjectRefFromObject(routeListener)
	logger.Info("Ensured RouteListener", "routeListener", routeListener.Name)

	// Step 10: Create bridge Subscribers.
	subRefs, err := h.ensureBridgeSubscribers(ctx, listener, publisher, appId,
		eventConfig.Status.CallbackURL, apiBasePath, consumerId, providerId)
	if err != nil {
		return errors.Wrap(err, "failed to ensure bridge Subscribers")
	}
	listener.Status.EventSubscriptions = subRefs
	logger.Info("Ensured bridge Subscribers", "count", len(subRefs))

	// Step 11: Set Ready condition.
	// AllReady() only turns false once a child reports Ready=False. A child that
	// was just created has no conditions at all, so check AnyChanged() first —
	// otherwise the first reconcile reports Ready before anything is confirmed.
	if c.AnyChanged() {
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"At least one sub-resource has been created or updated"))
		listener.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady,
			"At least one sub-resource has been created or updated"))
		return nil
	}

	if !c.AllReady() {
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		listener.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		return nil
	}

	listener.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned,
		"Listener has been provisioned"))
	listener.SetCondition(condition.NewDoneProcessingCondition("Listener has been provisioned"))

	return nil
}

func (h *ListenerHandler) Delete(ctx context.Context, listener *spectrev1.Listener) error {
	logger := log.FromContext(ctx)

	// The children live in the zone namespace while this Listener lives in the
	// team namespace. Kubernetes ignores cross-namespace owner references, so
	// nothing garbage collects them — they must be deleted explicitly from the
	// refs recorded in status.
	if err := h.deleteRouteListener(ctx, listener.Status.RouteListener); err != nil {
		return err
	}
	listener.Status.RouteListener = nil

	for i := range listener.Status.EventSubscriptions {
		ref := &listener.Status.EventSubscriptions[i]
		if err := h.deleteSubscriber(ctx, ref); err != nil {
			return err
		}
	}
	listener.Status.EventSubscriptions = nil

	// The shared generic Publisher is intentionally not owned by any single
	// Listener — it is ref-counted and removed once the last one goes away.
	zoneNamespace := h.publisherNamespace(ctx, listener)
	if zoneNamespace == "" {
		logger.V(1).Info("Could not determine zone namespace, skipping generic Publisher cleanup")
		return nil
	}

	if err := h.cleanupGenericPublisherIfOrphaned(ctx, listener, zoneNamespace); err != nil {
		return errors.Wrap(err, "failed to cleanup generic Publisher")
	}

	return nil
}

// publisherNamespace determines the zone namespace holding the shared generic
// Publisher. It prefers the namespace of an already-recorded child ref, which
// keeps cleanup working even after the referenced Applications are gone, and
// falls back to resolving the consumer Application's zone.
func (h *ListenerHandler) publisherNamespace(ctx context.Context, listener *spectrev1.Listener) string {
	logger := log.FromContext(ctx)

	if ref := listener.Status.RouteListener; ref != nil && ref.Namespace != "" {
		return ref.Namespace
	}
	for i := range listener.Status.EventSubscriptions {
		if ns := listener.Status.EventSubscriptions[i].Namespace; ns != "" {
			return ns
		}
	}

	consumerApp, err := h.resolveApplication(ctx, &listener.Spec.Consumer)
	if err != nil {
		logger.V(1).Info("Could not resolve consumer Application during delete", "error", err)
		return ""
	}
	consumerZone, err := h.resolveZone(ctx, consumerApp)
	if err != nil {
		logger.V(1).Info("Could not resolve zone during delete", "error", err)
		return ""
	}
	return consumerZone.Status.Namespace
}

// deleteRouteListener removes the RouteListener referenced in status, tolerating
// an already-deleted object.
func (h *ListenerHandler) deleteRouteListener(ctx context.Context, ref *ctypes.ObjectRef) error {
	if ref == nil {
		return nil
	}
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	rl := &gatewayv1.RouteListener{}
	if err := c.Get(ctx, ref.K8s(), rl); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get RouteListener %q", ref.String())
	}
	if err := c.Delete(ctx, rl); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete RouteListener %q", ref.String())
	}
	logger.Info("Deleted RouteListener", "routeListener", ref.String())
	return nil
}

// deleteSubscriber removes a bridge Subscriber referenced in status, tolerating
// an already-deleted object.
func (h *ListenerHandler) deleteSubscriber(ctx context.Context, ref *ctypes.ObjectRef) error {
	if ref == nil {
		return nil
	}
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	sub := &pubsubv1.Subscriber{}
	if err := c.Get(ctx, ref.K8s(), sub); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get Subscriber %q", ref.String())
	}
	if err := c.Delete(ctx, sub); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to delete Subscriber %q", ref.String())
	}
	logger.Info("Deleted bridge Subscriber", "subscriber", ref.String())
	return nil
}

// resolveApplication fetches an Application by TypedObjectRef and ensures it is ready.
func (h *ListenerHandler) resolveApplication(ctx context.Context, ref *ctypes.TypedObjectRef) (*applicationv1.Application, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	app := &applicationv1.Application{}
	err := c.Get(ctx, ref.K8s(), app)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q not found: %v", ref.ObjectRef.String(), err)
	}

	if err := condition.EnsureReady(app); err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q is not ready", ref.ObjectRef.String())
	}

	return app, nil
}

// resolveSpectreApplication fetches the SpectreApplication referenced by the Listener.
func (h *ListenerHandler) resolveSpectreApplication(ctx context.Context, listener *spectrev1.Listener) (*spectrev1.SpectreApplication, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	sa := &spectrev1.SpectreApplication{}
	if err := c.Get(ctx, listener.Spec.Application.K8s(), sa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ctrlerrors.BlockedErrorf("SpectreApplication %q not found", listener.Spec.Application.String())
		}
		return nil, errors.Wrapf(err, "failed to get SpectreApplication %q", listener.Spec.Application.String())
	}

	return sa, nil
}

// resolveZone fetches the Zone referenced by the Application and ensures it is ready.
func (h *ListenerHandler) resolveZone(ctx context.Context, app *applicationv1.Application) (*adminv1.Zone, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	zone := &adminv1.Zone{}
	err := c.Get(ctx, app.Spec.Zone.K8s(), zone)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q not found: %v", app.Spec.Zone.String(), err)
	}

	if err := condition.EnsureReady(zone); err != nil {
		return nil, ctrlerrors.BlockedErrorf("zone %q is not ready", app.Spec.Zone.String())
	}

	return zone, nil
}

// findEventStore lists EventStore CRs in the zone namespace and returns the single expected one.
func (h *ListenerHandler) findEventStore(ctx context.Context, zoneNamespace string) (*pubsubv1.EventStore, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	eventStoreList := &pubsubv1.EventStoreList{}
	err := c.List(ctx, eventStoreList, client.InNamespace(zoneNamespace))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list EventStores in namespace %q", zoneNamespace)
	}

	if len(eventStoreList.Items) == 0 {
		return nil, ctrlerrors.BlockedErrorf("no EventStore found in namespace %q", zoneNamespace)
	}

	return &eventStoreList.Items[0], nil
}

// findRouteByPath resolves the gateway Route that exposes the given apiBasePath.
//
// The Route is fetched by name rather than by matching Spec.Paths: the api domain
// derives the Route name deterministically from the base path
// (labelutil.NormalizeValue), whereas Spec.Paths holds the preset-joined paths
// (path.Join(preset.BasePath, apiBasePath)). Matching the raw apiBasePath against
// those only works in a zone whose gateway preset basePath is "/", and silently
// finds nothing otherwise.
//
// Returns (nil, nil) when no such Route exists; callers turn that into a
// BlockedError so the Listener waits for the Route to be provisioned.
func (h *ListenerHandler) findRouteByPath(ctx context.Context, namespace, apiBasePath string) (*gatewayv1.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	routeName := util.MakeRouteName(apiBasePath)
	route := &gatewayv1.Route{}
	err := c.Get(ctx, k8stypes.NamespacedName{Name: routeName, Namespace: namespace}, route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to get Route %q in namespace %q", routeName, namespace)
	}

	return route, nil
}

// ensureRouteListener creates or updates the RouteListener CR for this Listener.
func (h *ListenerHandler) ensureRouteListener(
	ctx context.Context,
	_ *spectrev1.Listener,
	zone *adminv1.Zone,
	appId string,
	consumerId string,
	providerId string,
	apiBasePath string,
) (*gatewayv1.RouteListener, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Resolve the actual gateway Route for this apiBasePath so the RouteListener
	// references the correct Route CR (the gateway handler queries by spec.route index).
	route, err := h.findRouteByPath(ctx, zone.Status.Namespace, apiBasePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find Route for apiBasePath")
	}

	var routeRef ctypes.ObjectRef
	if route != nil {
		routeRef = *ctypes.ObjectRefFromObject(route)
		logger.V(1).Info("Resolved Route for apiBasePath", "route", routeRef.String(), "apiBasePath", apiBasePath)
	} else {
		// Route not yet provisioned — block until it exists so the RouteListener
		// can be properly linked. The gateway handler uses a field index on spec.route
		// to discover RouteListeners, so a wrong reference means silent failure.
		return nil, ctrlerrors.BlockedErrorf("no Route found with path %q in namespace %q", apiBasePath, zone.Status.Namespace)
	}

	// Resolve the zone-level gateway client credentials. The jumper requires a
	// top-level gatewayClient with {id, issuer} derived from the zone's default
	// identity realm — not the consumer application's clientId.
	gwClientId, gwIssuer, err := h.resolveGatewayCredentials(ctx, zone)
	if err != nil {
		return nil, err
	}

	routeListenerName := util.MakeRouteListenerName(appId, apiBasePath, consumerId, providerId)
	rl := &gatewayv1.RouteListener{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeListenerName,
			Namespace: zone.Status.Namespace,
		},
	}

	mutator := func() error {
		rl.Spec = gatewayv1.RouteListenerSpec{
			Route: routeRef,
			Zone: ctypes.ObjectRef{
				Name:      zone.Name,
				Namespace: zone.Namespace,
			},
			Consumer:     consumerId,
			ServiceOwner: providerId,
			Issue:        apiBasePath,
			GatewayClient: gatewayv1.GatewayClientConfig{
				ClientId: gwClientId,
				Issuer:   gwIssuer,
			},
		}
		return nil
	}

	_, err = c.CreateOrUpdate(ctx, rl, mutator)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create or update RouteListener %q", routeListenerName)
	}

	return rl, nil
}

// resolveGatewayCredentials fetches the zone's default identity Realm and
// returns the gateway client id (zone-level singleton "gateway") and the
// realm's issuer URL. These are the values jumper needs to mint publisher tokens.
func (h *ListenerHandler) resolveGatewayCredentials(ctx context.Context, zone *adminv1.Zone) (clientId, issuer string, err error) {
	if zone.Status.IdentityRealm == nil {
		return "", "", ctrlerrors.BlockedErrorf("zone %q has no IdentityRealm in status", zone.Name)
	}

	c := cclient.ClientFromContextOrDie(ctx)
	realm := &identityv1.Realm{}
	if err := c.Get(ctx, zone.Status.IdentityRealm.K8s(), realm); err != nil {
		return "", "", errors.Wrapf(err, "failed to get identity Realm %q for zone %q", zone.Status.IdentityRealm.Name, zone.Name)
	}

	if realm.Status.IssuerUrl == "" {
		return "", "", ctrlerrors.BlockedErrorf("identity Realm %q has no IssuerUrl in status", realm.Name)
	}

	// "gateway" is the zone-level singleton consumer name — every listener on a
	// route resolves the same client, making this assignment idempotent.
	return "gateway", realm.Status.IssuerUrl, nil
}
