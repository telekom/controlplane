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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"fmt"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

type ListenerHandler struct {
	// Reader is the manager's uncached API reader (mgr.GetAPIReader()). Safety
	// decisions on current revocation and deletion state read through it, never
	// the cache. Handler unit tests and controller envtests that build the
	// handler directly leave it nil (getLive then reads through the scoped
	// client); SetupWithManager always sets it.
	Reader client.Reader
	// Now is the clock of the approval grace period. Nil uses time.Now; tests
	// inject a fake clock.
	Now func() time.Time
}

// authorizationUnknownGracePeriod is how long applied capture keeps running
// while the dual-gate evaluation gives no answer for a reason other than a
// scoped identity mismatch. Shorter outages leave capture untouched; after it
// capture is stopped through the persisted drain.
const authorizationUnknownGracePeriod = 5 * time.Minute

// reasonAuthorizationUnavailable is the Ready reason once the grace period has
// passed without an approval answer.
const reasonAuthorizationUnavailable = "AuthorizationUnavailable"

func (h *ListenerHandler) now() time.Time {
	if h.Now == nil {
		return time.Now()
	}
	return h.Now()
}

// getLive reads key through Reader for a safety decision. Reader lacks the
// ScopedClient's environment filtering, so its namespace default and
// environment-label check are applied here: a foreign object is a read error,
// never evidence. Without a Reader it reads through the scoped client.
func (h *ListenerHandler) getLive(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	if h.Reader == nil {
		return cclient.ClientFromContextOrDie(ctx).Get(ctx, key, obj)
	}
	env, ok := contextutil.EnvFromContext(ctx)
	if !ok {
		return errors.Errorf("no environment in context for the live read of %s", key)
	}
	if key.Namespace == "" {
		key.Namespace = env
	}
	if err := h.Reader.Get(ctx, key, obj); err != nil {
		return errors.Wrapf(err, "failed to read %s live", key)
	}
	labels := obj.GetLabels()
	if labels == nil || labels[cconfig.EnvironmentLabelKey] != env {
		return errors.Errorf("live object %s does not belong to the environment %q", key, env)
	}
	return nil
}

func (h *ListenerHandler) CreateOrUpdate(ctx context.Context, listener *spectrev1.Listener) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	// Step 0: Resume persisted drain using saved refs — no topology needed.
	// The drain checkpoint stores everything continueDrain needs (old child refs,
	// UIDs, publisher namespace). Running it before any topology resolution
	// ensures progress even when Applications/Zones/Routes are not ready.
	if listener.Status.Draining != nil {
		complete, err := h.continueDrain(ctx, listener)
		if err != nil {
			return errors.Wrap(err, "failed to continue drain")
		}
		if !complete {
			return nil // persist and requeue
		}
		// Drain complete: nothing is applied any more. A kept fingerprint would
		// make the early check below start the same drain again on every reconcile.
		if listener.Status.AppliedPlacement != nil {
			listener.Status.AppliedPlacement.Fingerprint = ""
		}
		// Fall through to resolve topology for new provisioning.
	}

	// Step 0.5: Early restriction check — detect a conclusive revocation, or a
	// rejected current request while capture is applied, from persisted status
	// refs without requiring Application/Route readiness, and drain even if
	// topology is broken. A read error does not mask a denial found by another read.
	approvalGate, requestGate, earlyErr := h.checkEarlyRestriction(ctx, listener)
	if earlyErr != nil {
		// Not returned: a denial found by another read still stops capture below.
		// Logged at V(0) because a persistent read failure (throttling, a foreign
		// object at a status ref) would otherwise disable this check silently.
		logger.Error(earlyErr, "Early restriction check could not read every gate; an unreadable gate cannot stop capture",
			"appliedCapture", hasAppliedCapture(listener))
	}
	switch {
	case approvalGate != "":
		logger.Info("Early restriction detected, initiating cleanup", "gate", approvalGate)
		if err := h.handleDenialCleanup(ctx, listener,
			fmt.Sprintf("early restriction (%s gate)", approvalGate),
			fmt.Sprintf("Approval has been revoked (%s gate, early restriction)", approvalGate)); err != nil {
			return errors.Wrap(err, "failed cleanup after early restriction")
		}
		return nil
	case requestGate != "":
		logger.Info("Rejected ApprovalRequest with applied capture, initiating cleanup", "gate", requestGate)
		if err := h.handleDenialCleanup(ctx, listener,
			fmt.Sprintf("approval request rejected (%s gate, early restriction)", requestGate),
			fmt.Sprintf("ApprovalRequest has been denied (%s gate, early restriction)", requestGate)); err != nil {
			return errors.Wrap(err, "failed cleanup after early restriction")
		}
		return nil
	}

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
	// live in other namespaces and therefore carry no owner references, so
	// provisioning under an empty appId would create wrongly named capture that a
	// later pass must drain and replace.
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

	// Reject event-only Listeners early — creating ApprovalRequests for a
	// Listener that can never provision downstream resources wastes effort and
	// leaves orphaned CRs.
	if listener.Spec.ApiListener == nil {
		return ctrlerrors.BlockedErrorf("Listener %q has no ApiListener configured (event-only listeners are not yet supported)", listener.Name)
	}
	apiBasePath := listener.Spec.ApiListener.ApiBasePath

	// The observer is the SpectreApplication's own Application (A), which may
	// differ from the consumer (C) when observing another team's traffic.
	observerApp, err := h.resolveApplication(ctx, &spectreApp.Spec.Application)
	if err != nil {
		return errors.Wrap(err, "failed to resolve observer Application")
	}
	observerZone, err := h.resolveZone(ctx, observerApp)
	if err != nil {
		return errors.Wrap(err, "failed to resolve observer zone")
	}

	// Step 4: Resolve placement before creating approvals. Delivery is always
	// A's zone; capture is the first supported zone on the C→P path (A's zone
	// when on it, then C's, then P's). Unsupported Routes (missing, pass-through,
	// failover, not bound to P) are skipped per candidate; when none remains,
	// applied capture is drained.
	lp, binding, err := h.resolveListenerPlacement(ctx, observerZone, consumerZone, providerZone, providerApp, apiBasePath)
	if err != nil {
		return h.handlePlacementError(ctx, listener, err)
	}

	// Compute the canonical authorization intent and fingerprint.
	placement := PlacementIntent{
		ApiExposureName:             binding.ApiExposureName,
		ApiExposureNamespace:        binding.ApiExposureNamespace,
		CaptureRouteName:            lp.CaptureRoute.Name,
		CaptureRouteNamespace:       lp.CaptureRoute.Namespace,
		CaptureZoneName:             lp.CaptureZone.Name,
		CaptureZoneNamespace:        lp.CaptureZone.Namespace,
		CaptureEventStoreName:       lp.CaptureEventStore.Name,
		CaptureEventStoreNamespace:  lp.CaptureEventStore.Namespace,
		CallbackOriginZoneName:      lp.CallbackOriginZone.Name,
		CallbackOriginZoneNamespace: lp.CallbackOriginZone.Namespace,
		DeliveryZoneName:            lp.DeliveryZone.Name,
		DeliveryZoneNamespace:       lp.DeliveryZone.Namespace,
		DeliveryEventStoreName:      lp.DeliveryEventStore.Name,
		DeliveryEventStoreNamespace: lp.DeliveryEventStore.Namespace,
		CallbackBaseURL:             lp.CallbackBaseURL,
	}
	intent := buildAuthorizationIntent(listener, consumerApp, providerApp, spectreApp, observerApp, placement)
	fingerprint := intent.fingerprint()

	// Step 5.9: Detect fingerprint change. If there are existing children with
	// a different fingerprint, start a drain to record what is being replaced.
	// startDrain only snapshots; the caller returns nil so the controller
	// persists the checkpoint before any destructive work begins.
	if listener.Status.AppliedPlacement != nil &&
		listener.Status.AppliedPlacement.Fingerprint != "" &&
		listener.Status.AppliedPlacement.Fingerprint != fingerprint &&
		listener.Status.Draining == nil {
		if err := h.startDrain(ctx, listener, "fingerprint changed", listener.Status.AppliedPlacement.Fingerprint); err != nil {
			return errors.Wrap(err, "failed to start drain")
		}
		return nil // persist drain checkpoint; continueDrain runs on next reconcile
	}

	// Step 5.10: Owned children the current intent did not produce (unlabelled
	// children, or another fingerprint step 5.9 did not catch) are drained
	// through the persisted checkpoint like every other capture stop; return so
	// it is persisted before continueDrain deletes anything. No drain is active
	// here: step 0 returns until it completes.
	started, staleErr := h.drainStaleChildren(ctx, listener, fingerprint)
	if staleErr != nil {
		return errors.Wrap(staleErr, "failed to drain stale children")
	}
	if started {
		return nil // persist drain checkpoint; continueDrain runs on next reconcile
	}

	// Step 6: Evaluate dual-gate approval (provider + consumer).
	dual, err := h.ensureApprovals(ctx, listener, observerApp, consumerApp, providerApp, &intent)
	if err != nil && dual == nil {
		return errors.Wrap(err, "failed to ensure approvals")
	}

	// Step 7: Handle approval states explicitly. Every answer (Granted, Denied,
	// RequestDenied, Pending) ends a no-answer grace period; a later failure to
	// answer starts a fresh one.
	switch dual.outcome {
	case outcomeGranted:
		listener.Status.AuthorizationUnknownSince = nil
		// Continue to provisioning below.

	case outcomeDenied, outcomeRequestDenied:
		listener.Status.AuthorizationUnknownSince = nil
		// A denied Approval stops capture through the persisted drain: RouteListeners
		// first (stop new traffic), then Subscribers. It is reached when the early
		// check could not see the denial (read error, no Approval ref persisted yet).
		//
		// Spectre policy (plan 2.3): a rejected current ApprovalRequest stops this
		// Listener's capture via the same persisted drain as an Approval denial.
		// The shared builder contract (RequestDenied = children untouched) is
		// unchanged for other domains. setAggregateConditions already set
		// Ready=False/AccessDenied naming the gate(s). The early check (step 0.5)
		// acts on a rejected request only while capture is applied; once the drain
		// has cleared it, an intent change (new request name) reaches ensureApprovals.
		reason := "approval denied"
		if dual.outcome == outcomeRequestDenied {
			reason = fmt.Sprintf("approval request rejected (%s gate)", requestDeniedGates(dual))
		}
		return h.stopCaptureAfter(ctx, listener, reason, dual.err)

	case outcomePending:
		listener.Status.AuthorizationUnknownSince = nil
		// No new provisioning; no stale child remains (step 5.10).
		if dual.err != nil {
			return errors.Wrap(dual.err, "combined approval error")
		}
		return nil

	case outcomeError:
		evalErr := dual.err
		if evalErr == nil {
			evalErr = errors.New("no gate diagnostics")
		}
		evalErr = errors.Wrap(evalErr, "approval evaluation failed")
		// A definitive scoped identity failure (ownerless or foreign Approval or
		// request) is permanent: the approval controller never repairs it, so no
		// applied capture may keep running on it. Any other failure gives no
		// answer: capture keeps running for the grace period only.
		if gate := scopedIdentityGates(dual); gate != "" {
			return h.stopCaptureAfter(ctx, listener, fmt.Sprintf("approval identity invalid (%s gate)", gate), evalErr)
		}
		return h.awaitApprovalAnswer(ctx, listener, dual, evalErr)

	default:
		// outcomeUnknown: fail closed.
		return errors.Errorf("unhandled approval outcome %d", dual.outcome)
	}

	logger.Info("Approval granted, provisioning downstream resources")

	// Step 8: Ensure shared generic Publisher.
	publisher, err := h.ensureGenericPublisher(ctx, lp.CaptureEventStore)
	if err != nil {
		return errors.Wrap(err, "failed to ensure generic Publisher")
	}
	logger.Info("Ensured generic Publisher", "publisher", publisher.Name)

	// Step 9: Create RouteListener.
	routeListener, err := h.ensureRouteListener(ctx, listener, lp.CaptureZone, lp.CaptureRoute, appId, consumerId, providerId, apiBasePath, fingerprint)
	if err != nil {
		return errors.Wrap(err, "failed to ensure RouteListener")
	}
	listener.Status.RouteListener = ctypes.ObjectRefFromObject(routeListener)
	logger.Info("Ensured RouteListener", "routeListener", routeListener.Name)

	// Step 10: Create bridge Subscribers.
	subRefs, err := h.ensureBridgeSubscribers(ctx, listener, publisher, appId,
		lp.CallbackBaseURL, apiBasePath, consumerId, providerId, fingerprint)
	if err != nil {
		return errors.Wrap(err, "failed to ensure bridge Subscribers")
	}
	listener.Status.EventSubscriptions = subRefs
	logger.Info("Ensured bridge Subscribers", "count", len(subRefs))

	// Step 10.5: Update applied placement now that all children are provisioned.
	listener.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{
		Fingerprint:        fingerprint,
		CaptureRoute:       ctypes.ObjectRefFromObject(lp.CaptureRoute),
		CaptureZone:        ctypes.ObjectRefFromObject(lp.CaptureZone),
		CaptureEventStore:  ctypes.ObjectRefFromObject(lp.CaptureEventStore),
		DeliveryZone:       ctypes.ObjectRefFromObject(lp.DeliveryZone),
		DeliveryEventStore: ctypes.ObjectRefFromObject(lp.DeliveryEventStore),
		CallbackOriginZone: ctypes.ObjectRefFromObject(lp.CallbackOriginZone),
		Publisher:          ctypes.ObjectRefFromObject(publisher),
		CallbackBaseURL:    lp.CallbackBaseURL,
	}

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

	// Guard: AllReady() treats children with no conditions as ready (it only
	// flips on Ready=False). Explicitly verify each required child carries an
	// actual Ready=True condition before declaring the parent ready.
	if err := ensureChildReady(ctx, listener.Status.RouteListener, &gatewayv1.RouteListener{}); err != nil {
		listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, err.Error()))
		listener.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady, err.Error()))
		return nil
	}
	for i := range listener.Status.EventSubscriptions {
		ref := &listener.Status.EventSubscriptions[i]
		if err := ensureChildReady(ctx, ref, &pubsubv1.Subscriber{}); err != nil {
			listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonSubResourceNotReady, err.Error()))
			listener.SetCondition(condition.NewProcessingCondition(condition.ReasonSubResourceNotReady, err.Error()))
			return nil
		}
	}

	listener.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned,
		"Listener has been provisioned"))
	listener.SetCondition(condition.NewDoneProcessingCondition("Listener has been provisioned"))

	return nil
}

// Delete stops capture through the same persisted drain as CreateOrUpdate:
// checkpoint, RouteListeners, Subscribers, then the Publisher in every old
// namespace. The common controller removes the finalizer only when Delete
// returns nil and writes status only when it returns an error, so every pass
// that must be persisted returns errDrainPending.
func (h *ListenerHandler) Delete(ctx context.Context, listener *spectrev1.Listener) error {
	if listener.Status.Draining != nil {
		complete, err := h.continueDrain(ctx, listener)
		if err != nil {
			return errors.Wrap(err, "failed to continue drain")
		}
		if !complete {
			return errDrainPending(listener)
		}
		// Nothing drained is applied any more; a kept fingerprint would restart it.
		if ap := listener.Status.AppliedPlacement; ap != nil {
			ap.Fingerprint = ""
		}
	}
	// A fresh inventory: release only when no owned child remains.
	started, err := h.drainCapture(ctx, listener, "listener deleted")
	if err != nil {
		return errors.Wrap(err, "failed to start drain")
	}
	if started {
		return errDrainPending(listener)
	}
	return nil
}

// errDrainPending marks the Listener not ready with the drain phase deletion
// waits on, keeps the finalizer, and has the common controller persist the
// drain checkpoint and retry shortly. The condition is set on every pass: the
// controller's own NotReady write skips a Ready that is already False, which
// would freeze the first phase or keep a pre-deletion reason such as AccessDenied.
func errDrainPending(listener *spectrev1.Listener) error {
	phase := listener.Status.Draining.Phase
	listener.SetCondition(condition.NewNotReadyCondition("Deleting",
		fmt.Sprintf("Draining capture before deletion (phase %s)", phase)))
	return ctrlerrors.RetryableWithDelayErrorf(2*time.Second, "draining capture before deletion (phase %s)", phase)
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

// checkEarlyRestriction reads the Approvals referenced by the Listener's
// persisted status and returns the gate of one that is conclusively revoked
// (Rejected, Suspended, or Expired-from-Suspended). While capture is applied it
// also reads the referenced current ApprovalRequests and returns the gate of a
// Rejected one; a ref with a UID must match the live request, so NotFound or a
// recreated request is not a rejection. This is a READ-ONLY check that works
// even when the Application/Route topology is temporarily unreachable. Reads
// are live (getLive), so a revocation the cache has not seen yet still counts.
// Every read is attempted; the first read error is returned with any denial found.
func (h *ListenerHandler) checkEarlyRestriction(
	ctx context.Context,
	listener *spectrev1.Listener,
) (approvalGate, requestGate string, err error) {
	var firstErr error
	get := func(ref *ctypes.ObjectRef, obj client.Object) bool {
		if err := h.getLive(ctx, ref.K8s(), obj); err != nil {
			// Missing = not denied (might be pending creation). Record other errors
			// but try the next read — denial takes priority.
			if !apierrors.IsNotFound(err) && firstErr == nil {
				firstErr = err
			}
			return false
		}
		return true
	}

	gates := []struct {
		key      string
		approval *ctypes.ObjectRef
		request  *ctypes.ObjectRef
	}{
		{"provider", listener.Status.ProviderApproval, listener.Status.ProviderApprovalRequest},
		{"consumer", listener.Status.ConsumerApproval, listener.Status.ConsumerApprovalRequest},
	}
	for _, gate := range gates {
		approval := &approvalapi.Approval{}
		if gate.approval == nil || !get(gate.approval, approval) {
			continue
		}
		if approval.Spec.State == approvalapi.ApprovalStateRejected ||
			approval.Spec.State == approvalapi.ApprovalStateSuspended {
			return gate.key, "", firstErr
		}
		// Expired-from-Suspended = also denied.
		if approval.Spec.State == approvalapi.ApprovalStateExpired &&
			approval.Status.LastState == approvalapi.ApprovalStateSuspended {
			return gate.key, "", firstErr
		}
	}

	// With nothing applied a rejected request stops nothing, and the normal flow
	// must still reach ensureApprovals for a changed intent.
	if !hasAppliedCapture(listener) {
		return "", "", firstErr
	}
	for _, gate := range gates {
		request := &approvalapi.ApprovalRequest{}
		if gate.request == nil || !get(gate.request, request) {
			continue
		}
		if gate.request.UID != "" && request.UID != gate.request.UID {
			continue
		}
		if request.Spec.State == approvalapi.ApprovalStateRejected {
			return "", gate.key, firstErr
		}
	}
	return "", "", firstErr
}

// hasAppliedCapture reports whether capture may still run with no drain stopping
// it: an applied fingerprint, or status refs of capture children. The refs
// cover partial provisioning, because they are written before AppliedPlacement.
func hasAppliedCapture(listener *spectrev1.Listener) bool {
	if listener.Status.Draining != nil {
		return false
	}
	ap := listener.Status.AppliedPlacement
	return (ap != nil && ap.Fingerprint != "") ||
		listener.Status.RouteListener != nil ||
		len(listener.Status.EventSubscriptions) > 0
}

// handleDenialCleanup stops capture through the persisted drain and sets
// AccessDenied conditions with message when the early check detects a
// conclusive revocation or a rejected current request.
func (h *ListenerHandler) handleDenialCleanup(
	ctx context.Context,
	listener *spectrev1.Listener,
	reason string,
	message string,
) error {
	if _, err := h.drainCapture(ctx, listener, reason); err != nil {
		return errors.Wrap(err, "failed to start drain during denial cleanup")
	}

	listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonAccessDenied, message))
	listener.SetCondition(condition.NewDoneProcessingCondition(message))
	return nil
}

// stopCaptureAfter stops capture through the persisted drain after an approval
// decision and returns the cleanup failure joined with approvalErr, or
// approvalErr. The caller returns it, so the checkpoint is persisted before
// continueDrain deletes anything.
func (h *ListenerHandler) stopCaptureAfter(
	ctx context.Context,
	listener *spectrev1.Listener,
	reason string,
	approvalErr error,
) error {
	_, stopErr := h.drainCapture(ctx, listener, reason)
	return joinStopError(reason, stopErr, approvalErr)
}

// joinStopError returns the failure to stop capture after reason joined with
// approvalErr, or approvalErr.
func joinStopError(reason string, stopErr, approvalErr error) error {
	if stopErr != nil && approvalErr != nil {
		return fmt.Errorf("failed to stop capture after %s: %w; combined approval error: %w", reason, stopErr, approvalErr)
	}
	if stopErr != nil {
		return errors.Wrapf(stopErr, "failed to stop capture after %s", reason)
	}
	if approvalErr != nil {
		return errors.Wrap(approvalErr, "combined approval error")
	}
	return nil
}

// awaitApprovalAnswer handles a dual-gate evaluation that gave no answer.
// Applied capture keeps running for authorizationUnknownGracePeriod from the
// first such reconcile, recorded in status.authorizationUnknownSince. Within
// it the Listener is retried after a delay, because a failing read may produce
// no watch event to wake it. The handler never asks for more than the time
// left in the period, but the controller's jitter stretches each delay by up
// to JitterFactor, so the stop can come up to JitterFactor x the 30s step
// (about 21s with the defaults) after the period ends. After it, capture is
// stopped through the persisted drain and the Listener stays not ready until
// both gates answer. Nothing new is provisioned either way.
func (h *ListenerHandler) awaitApprovalAnswer(
	ctx context.Context,
	listener *spectrev1.Listener,
	dual *dualApprovalResult,
	evalErr error,
) error {
	now := h.now()
	if listener.Status.AuthorizationUnknownSince == nil {
		listener.Status.AuthorizationUnknownSince = &metav1.Time{Time: now}
	}
	since := listener.Status.AuthorizationUnknownSince.Time
	// Messages name the start time, not the elapsed time, so the persisted
	// status does not change on every retry.
	sinceText := since.UTC().Format(time.RFC3339)
	if remaining := since.Add(authorizationUnknownGracePeriod).Sub(now); remaining > 0 {
		return ctrlerrors.RetryableWithDelayErrorf(min(remaining, 30*time.Second),
			"%v; no approval answer since %s, applied capture is kept for %s from then",
			evalErr, sinceText, authorizationUnknownGracePeriod)
	}

	gates := gateNames(dual, func(g *gateResult) bool {
		return g.outcome == outcomeError || g.outcome == outcomeUnknown
	})
	reason := fmt.Sprintf("approval answer unavailable (%s gate)", gates)
	stopping, stopErr := h.drainCapture(ctx, listener, reason)
	// Only an inventory that found nothing may say no capture runs.
	outcome := "no capture runs and none is provisioned until both gates answer"
	if stopping || stopErr != nil {
		outcome = fmt.Sprintf("capture is stopped after %s without an answer", authorizationUnknownGracePeriod)
	}
	message := fmt.Sprintf("No approval answer for the %s gate since %s; %s", gates, sinceText, outcome)
	listener.SetCondition(condition.NewNotReadyCondition(reasonAuthorizationUnavailable, message))
	listener.SetCondition(condition.NewBlockedCondition(message))
	return joinStopError(reason, stopErr, evalErr)
}
