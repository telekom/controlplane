// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
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
	// The appId becomes part of the RouteListener and Subscriber names and of the
	// bridge callback URL, so it is effectively immutable identity. Those children
	// live in other namespaces and therefore carry no owner references, and Delete
	// only removes what the Listener status currently points at — so provisioning
	// under an empty appId leaves untracked resources behind once a later pass
	// creates the correctly named ones.
	appId := spectreApp.Status.Id
	if appId == "" {
		return ctrlerrors.BlockedErrorf("SpectreApplication %q has not resolved its application id yet",
			listener.Spec.Application.String())
	}

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

	// Step 5: Resolve EventStore via EventConfig reference.
	eventStore, err := util.ResolveEventStore(ctx, eventConfig)
	if err != nil {
		return errors.Wrap(err, "failed to resolve EventStore")
	}

	// Reject event-only Listeners early — creating ApprovalRequests for a
	// Listener that can never provision downstream resources wastes effort and
	// leaves orphaned CRs.
	if listener.Spec.ApiListener == nil {
		return ctrlerrors.BlockedErrorf("Listener %q has no ApiListener configured (event-only listeners are not yet supported)", listener.Name)
	}
	apiBasePath := listener.Spec.ApiListener.ApiBasePath

	// Step 5.5: Resolve the gateway Route early so unsupported modes
	// (pass-through, failover) are rejected before creating approvals.
	route, err := h.findRouteByPath(ctx, listeningZone.Status.Namespace, apiBasePath)
	if err != nil {
		return errors.Wrap(err, "failed to find Route for apiBasePath")
	}
	if route == nil {
		return ctrlerrors.BlockedErrorf("no Route found with path %q in namespace %q", apiBasePath, listeningZone.Status.Namespace)
	}

	// Step 5.6: Reject routes whose mode is incompatible with listener capture.
	// Pass-through routes skip authentication entirely (the route handler wraps
	// RouteListener collection in `if !route.Spec.PassThrough`), and failover
	// routes overwrite the /listener upstream to /proxy (priority 109 > 103).
	// In either case, clean up any existing children first — this handles a
	// previously-supported Route that changed to an unsupported mode.
	if route.Spec.PassThrough || route.Spec.Traffic.Failover != nil {
		if err := h.deleteAllOwnedChildren(ctx, listener); err != nil {
			return errors.Wrap(err, "failed to cleanup children for unsupported route mode")
		}
		listener.Status.RouteListener = nil
		listener.Status.EventSubscriptions = nil

		mode := "pass-through"
		if route.Spec.Traffic.Failover != nil {
			mode = "failover"
		}
		return ctrlerrors.BlockedErrorf("Route %q is %s — listener capture is not supported for this route mode", route.Name, mode)
	}

	// Step 5.7: Compute the canonical authorization intent and fingerprint.
	intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp)
	fingerprint := intent.fingerprint()

	// Step 5.8: Remove stale children whose fingerprint differs from the current
	// intent BEFORE evaluating the replacement grant. This ensures a provider,
	// application, path, delivery, or capture-scope change stops the old capture
	// immediately. Unlabelled children (pre-migration) are treated as stale.
	if err := h.removeStaleChildren(ctx, listener, fingerprint); err != nil {
		return errors.Wrap(err, "failed to remove stale children")
	}

	// Step 6: Create the provider approval (gate).
	approval, err := h.ensureApprovals(ctx, listener, consumerApp, providerApp, &intent)
	if err != nil {
		return errors.Wrap(err, "failed to ensure approvals")
	}

	listener.Status.ProviderApproval = approval.providerApproval

	// Step 7: Handle approval states explicitly.
	switch approval.result {
	case builder.ApprovalResultGranted:
		// Continue to provisioning below.

	case builder.ApprovalResultDenied:
		// Delete all owner-labelled capture children: RouteListeners first (stop
		// new traffic), then Subscribers.
		if err := h.deleteAllOwnedChildren(ctx, listener); err != nil {
			return errors.Wrap(err, "failed to cleanup children after denial")
		}
		listener.Status.RouteListener = nil
		listener.Status.EventSubscriptions = nil
		return nil

	case builder.ApprovalResultPending:
		// Do not provision; stale children already removed above.
		return nil

	case builder.ApprovalResultRequestDenied:
		// Do not provision; retain same-intent children (matching the builder
		// "do not touch current children" contract). Stale children were already
		// removed in step 5.6.
		return nil

	default:
		// ensureApprovals already returns an error for unknown results, but
		// defend against future additions.
		return errors.Errorf("unhandled approval result %q", approval.result)
	}

	logger.Info("Approval granted, provisioning downstream resources")

	// Step 8: Ensure shared generic Publisher.
	publisher, err := h.ensureGenericPublisher(ctx, eventStore)
	if err != nil {
		return errors.Wrap(err, "failed to ensure generic Publisher")
	}
	logger.Info("Ensured generic Publisher", "publisher", publisher.Name)

	// Step 9: Create RouteListener.
	routeListener, err := h.ensureRouteListener(ctx, listener, listeningZone, route, appId, consumerId, providerId, apiBasePath, fingerprint)
	if err != nil {
		return errors.Wrap(err, "failed to ensure RouteListener")
	}
	listener.Status.RouteListener = ctypes.ObjectRefFromObject(routeListener)
	logger.Info("Ensured RouteListener", "routeListener", routeListener.Name)

	// Step 10: Create bridge Subscribers.
	subRefs, err := h.ensureBridgeSubscribers(ctx, listener, publisher, appId,
		eventConfig.Status.CallbackURL, apiBasePath, consumerId, providerId, fingerprint)
	if err != nil {
		return errors.Wrap(err, "failed to ensure bridge Subscribers")
	}
	listener.Status.EventSubscriptions = subRefs
	logger.Info("Ensured bridge Subscribers", "count", len(subRefs))

	// Step 11: Janitor cleanup — remove any extra owner-labelled children that
	// were not touched during this reconcile (e.g. leftover from a name change).
	if _, err := c.Cleanup(ctx, &gatewayv1.RouteListenerList{}, cclient.OwnedByLabel(listener)); err != nil {
		return errors.Wrap(err, "failed to cleanup extra RouteListeners")
	}
	if _, err := c.Cleanup(ctx, &pubsubv1.SubscriberList{}, cclient.OwnedByLabel(listener)); err != nil {
		return errors.Wrap(err, "failed to cleanup extra Subscribers")
	}

	// Step 12: Set Ready condition.
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
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Phase 0: Resolve the publisher namespace BEFORE clearing any status.
	// The namespace comes from status refs, then owner-labelled children, then
	// topology as a last resort.
	zoneNamespace, err := h.resolvePublisherNamespace(ctx, listener)
	if err != nil {
		return errors.Wrap(err, "failed to resolve publisher namespace")
	}

	// Phase 1: Delete RouteListener first to stop new capture.
	if err := h.deleteRouteListener(ctx, listener.Status.RouteListener); err != nil {
		return err
	}
	// Delete any owner-labelled RouteListeners the status missed.
	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned RouteListeners")
	}
	for i := range rlList.Items {
		rl := &rlList.Items[i]
		if err := c.Delete(ctx, rl); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete RouteListener %q", rl.Name)
		}
		logger.Info("Deleted owned RouteListener", "routeListener", rl.Name, "namespace", rl.Namespace)
	}
	listener.Status.RouteListener = nil

	// Phase 2: Request deletion of bridge Subscribers from status refs.
	for i := range listener.Status.EventSubscriptions {
		ref := &listener.Status.EventSubscriptions[i]
		if err := h.deleteSubscriber(ctx, ref); err != nil {
			return err
		}
	}
	// Delete any owner-labelled Subscribers the status missed.
	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned Subscribers")
	}
	for i := range subList.Items {
		sub := &subList.Items[i]
		if err := c.Delete(ctx, sub); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete Subscriber %q", sub.Name)
		}
		logger.Info("Deleted owned Subscriber", "subscriber", sub.Name, "namespace", sub.Namespace)
	}

	// Phase 3: Fresh-list owner-labelled Subscribers. If any remain (finalizers
	// still running), retry so the Publisher is not deleted prematurely.
	remainingList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, remainingList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to re-list owned Subscribers")
	}
	if len(remainingList.Items) > 0 {
		return ctrlerrors.RetryableWithDelayErrorf(
			2*time.Second,
			"waiting for bridge Subscriber finalization before deleting generic Publisher",
		)
	}

	// Phase 4: All Subscribers gone — clear subscriber status.
	listener.Status.EventSubscriptions = nil

	// Phase 5: Decide whether the shared generic Publisher is unused.
	if zoneNamespace == "" {
		logger.V(1).Info("Could not determine zone namespace, skipping generic Publisher cleanup")
		return nil
	}

	if err := h.cleanupGenericPublisherIfOrphaned(ctx, zoneNamespace); err != nil {
		return errors.Wrap(err, "failed to cleanup generic Publisher")
	}

	return nil
}

// removeStaleChildren lists all Listener-owned RouteListeners and Subscribers
// and deletes those whose authorization fingerprint differs from the current
// intent. Resources without the fingerprint label (pre-migration) are treated
// as stale to enforce fail-closed migration.
//
// RouteListeners are deleted before Subscribers so no new traffic is captured
// while the replacement approval is pending.
func (h *ListenerHandler) removeStaleChildren(ctx context.Context, listener *spectrev1.Listener, currentFingerprint string) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// List all owner-labelled RouteListeners.
	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned RouteListeners")
	}
	for i := range rlList.Items {
		rl := &rlList.Items[i]
		if isStaleChild(rl.Labels, currentFingerprint) {
			logger.Info("Deleting stale RouteListener", "routeListener", rl.Name, "namespace", rl.Namespace)
			if err := c.Delete(ctx, rl); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete stale RouteListener %q", rl.Name)
			}
			// Clear status ref if it points to this stale child.
			if listener.Status.RouteListener != nil && listener.Status.RouteListener.Name == rl.Name && listener.Status.RouteListener.Namespace == rl.Namespace {
				listener.Status.RouteListener = nil
			}
		}
	}

	// List all owner-labelled Subscribers.
	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned Subscribers")
	}
	for i := range subList.Items {
		sub := &subList.Items[i]
		if isStaleChild(sub.Labels, currentFingerprint) {
			logger.Info("Deleting stale Subscriber", "subscriber", sub.Name, "namespace", sub.Namespace)
			if err := c.Delete(ctx, sub); err != nil && !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to delete stale Subscriber %q", sub.Name)
			}
		}
	}
	// Clear status refs that pointed to stale Subscribers.
	if len(subList.Items) > 0 {
		var kept []ctypes.ObjectRef
		for _, ref := range listener.Status.EventSubscriptions {
			stale := false
			for i := range subList.Items {
				sub := &subList.Items[i]
				if ref.Name == sub.Name && ref.Namespace == sub.Namespace && isStaleChild(sub.Labels, currentFingerprint) {
					stale = true
					break
				}
			}
			if !stale {
				kept = append(kept, ref)
			}
		}
		listener.Status.EventSubscriptions = kept
	}

	return nil
}

// deleteAllOwnedChildren removes all owner-labelled RouteListeners and
// Subscribers. Used on the Denied path where capture must stop entirely.
// RouteListeners are deleted first so no new traffic is captured.
func (h *ListenerHandler) deleteAllOwnedChildren(ctx context.Context, listener *spectrev1.Listener) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Delete RouteListeners first.
	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned RouteListeners for denial cleanup")
	}
	for i := range rlList.Items {
		rl := &rlList.Items[i]
		logger.Info("Deleting RouteListener after denial", "routeListener", rl.Name, "namespace", rl.Namespace)
		if err := c.Delete(ctx, rl); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete RouteListener %q after denial", rl.Name)
		}
	}

	// Then Subscribers.
	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned Subscribers for denial cleanup")
	}
	for i := range subList.Items {
		sub := &subList.Items[i]
		logger.Info("Deleting Subscriber after denial", "subscriber", sub.Name, "namespace", sub.Namespace)
		if err := c.Delete(ctx, sub); err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to delete Subscriber %q after denial", sub.Name)
		}
	}

	return nil
}

// resolvePublisherNamespace determines the zone namespace holding the shared
// generic Publisher. It uses a preference order:
//  1. namespace from status RouteListener/Subscriber refs
//  2. namespace from owner-labelled children (status-update-failure recovery)
//  3. current topology resolution as a final fallback
//
// Returns an error if owner-labelled children span multiple namespaces.
func (h *ListenerHandler) resolvePublisherNamespace(ctx context.Context, listener *spectrev1.Listener) (string, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Preference 1: status refs.
	if ref := listener.Status.RouteListener; ref != nil && ref.Namespace != "" {
		return ref.Namespace, nil
	}
	for i := range listener.Status.EventSubscriptions {
		if ns := listener.Status.EventSubscriptions[i].Namespace; ns != "" {
			return ns, nil
		}
	}

	// Preference 2: owner-labelled children.
	namespaces := make(map[string]struct{})

	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return "", errors.Wrap(err, "failed to list owned RouteListeners for namespace resolution")
	}
	for i := range rlList.Items {
		namespaces[rlList.Items[i].Namespace] = struct{}{}
	}

	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return "", errors.Wrap(err, "failed to list owned Subscribers for namespace resolution")
	}
	for i := range subList.Items {
		namespaces[subList.Items[i].Namespace] = struct{}{}
	}

	if len(namespaces) == 1 {
		for ns := range namespaces {
			return ns, nil
		}
	}
	if len(namespaces) > 1 {
		return "", errors.Errorf("owner-labelled children span multiple namespaces: found %d distinct namespaces", len(namespaces))
	}

	// Preference 3: topology resolution (consumer zone).
	consumerApp, err := h.resolveApplication(ctx, &listener.Spec.Consumer)
	if err != nil {
		logger.V(1).Info("Could not resolve consumer Application during delete", "error", err)
		return "", nil
	}
	consumerZone, err := h.resolveZone(ctx, consumerApp)
	if err != nil {
		logger.V(1).Info("Could not resolve zone during delete", "error", err)
		return "", nil
	}
	return consumerZone.Status.Namespace, nil
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
// The Route is resolved and validated by the caller (CreateOrUpdate) before
// approval evaluation, so this method receives it pre-resolved.
func (h *ListenerHandler) ensureRouteListener(
	ctx context.Context,
	listener *spectrev1.Listener,
	zone *adminv1.Zone,
	route *gatewayv1.Route,
	appId string,
	consumerId string,
	providerId string,
	apiBasePath string,
	fingerprint string,
) (*gatewayv1.RouteListener, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	routeRef := *ctypes.ObjectRefFromObject(route)
	logger.V(1).Info("Resolved Route for apiBasePath", "route", routeRef.String(), "apiBasePath", apiBasePath)

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
		if rl.Labels == nil {
			rl.Labels = make(map[string]string)
		}
		rl.Labels[cconfig.OwnerUidLabelKey] = string(listener.UID)
		rl.Labels[AuthorizationFingerprintLabelKey] = fingerprint
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
