// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

type ListenerHandler struct{}

func (h *ListenerHandler) CreateOrUpdate(ctx context.Context, listener *spectrev1.Listener) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Step 1: Resolve consumer and provider Applications.
	consumerApp, err := h.resolveApplication(ctx, listener.Spec.Consumer)
	if err != nil {
		return errors.Wrap(err, "failed to resolve consumer Application")
	}
	providerApp, err := h.resolveApplication(ctx, listener.Spec.Provider)
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

	listeningZone, err := util.GetListeningZone(ctx, consumerZone, providerZone, consumerZone)
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

	// Step 6: Create approvals (gate).
	listenerTeam := consumerApp.Spec.Team
	listenerEmail := consumerApp.Spec.TeamEmail

	approval, err := h.ensureApprovals(ctx, listener,
		listenerTeam, listenerEmail,
		providerApp.Spec.Team, providerApp.Spec.TeamEmail,
		consumerApp.Spec.Team, consumerApp.Spec.TeamEmail)
	if err != nil {
		return errors.Wrap(err, "failed to ensure approvals")
	}

	listener.Status.ProviderApproval = approval.providerApproval
	listener.Status.ConsumerApproval = approval.consumerApproval

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
	if listener.Spec.ApiListener == nil {
		return ctrlerrors.BlockedErrorf("Listener %q has no ApiListener configured", listener.Name)
	}
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
	if !c.AllReady() {
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady,
			"One or more child resources are not yet ready"))
		return nil
	}

	listener.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned,
		"Listener has been provisioned"))

	return nil
}

func (h *ListenerHandler) Delete(ctx context.Context, listener *spectrev1.Listener) error {
	logger := log.FromContext(ctx)

	// Owner-referenced children (RouteListener, bridge Subscribers) cascade via K8s.
	// The shared generic Publisher is NOT owner-referenced — we ref-count it.
	consumerApp, err := h.resolveApplication(ctx, listener.Spec.Consumer)
	if err != nil {
		// If we cannot resolve the app (e.g., it was already deleted), try zone from status
		logger.V(1).Info("Could not resolve consumer Application during delete, skipping Publisher cleanup", "error", err)
		return nil
	}

	consumerZone, err := h.resolveZone(ctx, consumerApp)
	if err != nil {
		logger.V(1).Info("Could not resolve zone during delete, skipping Publisher cleanup", "error", err)
		return nil
	}

	if err := h.cleanupGenericPublisherIfOrphaned(ctx, listener, consumerZone.Status.Namespace); err != nil {
		return errors.Wrap(err, "failed to cleanup generic Publisher")
	}

	return nil
}

// resolveApplication fetches an Application by TypedObjectRef and ensures it is ready.
func (h *ListenerHandler) resolveApplication(ctx context.Context, ref ctypes.TypedObjectRef) (*applicationv1.Application, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	app := &applicationv1.Application{}
	err := c.Get(ctx, ref.ObjectRef.K8s(), app)
	if err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q not found: %v", ref.ObjectRef.String(), err)
	}

	if err := condition.EnsureReady(app); err != nil {
		return nil, ctrlerrors.BlockedErrorf("application %q is not ready", ref.ObjectRef.String())
	}

	return app, nil
}

// resolveSpectreApplication finds the SpectreApplication that owns this Listener.
// It looks for a SpectreApplication in the same namespace with a matching owner reference,
// or falls back to listing SpectreApplications in the namespace.
func (h *ListenerHandler) resolveSpectreApplication(ctx context.Context, listener *spectrev1.Listener) (*spectrev1.SpectreApplication, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	// List SpectreApplications in the same namespace
	saList := &spectrev1.SpectreApplicationList{}
	err := c.List(ctx, saList, client.InNamespace(listener.Namespace))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list SpectreApplications in namespace %q", listener.Namespace)
	}

	if len(saList.Items) == 0 {
		return nil, ctrlerrors.BlockedErrorf("no SpectreApplication found in namespace %q", listener.Namespace)
	}

	// Use the first ready SpectreApplication
	for i := range saList.Items {
		sa := &saList.Items[i]
		if sa.Status.Id != "" {
			return sa, nil
		}
	}

	return &saList.Items[0], nil
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

// findRouteByPath lists gateway Routes in the given namespace and returns the first one
// whose Spec.Paths contains the apiBasePath. This is how we resolve which Route CR
// the RouteListener should attach to.
func (h *ListenerHandler) findRouteByPath(ctx context.Context, namespace string, apiBasePath string) (*gatewayv1.Route, error) {
	c := cclient.ClientFromContextOrDie(ctx)

	routeList := &gatewayv1.RouteList{}
	err := c.List(ctx, routeList, client.InNamespace(namespace))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list Routes in namespace %q", namespace)
	}

	for i := range routeList.Items {
		for _, p := range routeList.Items[i].Spec.Paths {
			if p == apiBasePath {
				return &routeList.Items[i], nil
			}
		}
	}

	return nil, nil
}

// ensureRouteListener creates or updates the RouteListener CR for this Listener.
func (h *ListenerHandler) ensureRouteListener(
	ctx context.Context,
	listener *spectrev1.Listener,
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
				// TODO(O5): Resolve actual gateway client credentials from zone or EventConfig
				ClientId: consumerId,
				Issuer:   "https://iris.telekom.de",
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
