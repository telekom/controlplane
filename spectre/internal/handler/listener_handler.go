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
		// Drain complete: nothing is applied any more, also while the migration
		// drains (it only advances from here). A kept fingerprint would make the
		// early check below start the same drain again on every reconcile.
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
		logger.V(1).Info("Early restriction check failed", "error", earlyErr)
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
		// Like every other drain, this one follows the freshness decision, so a
		// prior-policy Listener it drains never looks fresh afterwards.
		if err := h.decideFreshness(ctx, listener); err != nil {
			return err
		}
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

	// Step 3.5: Decide freshness before placement, fingerprint or stale-child
	// handling can drain a child: a Listener whose prior-policy children were
	// drained must not then look fresh. The early restriction (step 0.5) drains
	// on a revoked Approval only through a persisted Approval ref, and an unscoped
	// legacy ref keeps the Listener not fresh; on a rejected request it decides
	// freshness itself first.
	if freshErr := h.decideFreshness(ctx, listener); freshErr != nil {
		return freshErr
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
	//
	// SKIP when migration is in Draining phase: the drain was initiated by
	// advanceMigration and will be consumed there. Re-triggering here would
	// overwrite the migration's DrainStarted checkpoint before advanceMigration
	// can advance to RetiringRequests.
	migrationIsDraining := listener.Status.AuthorizationMigration != nil &&
		listener.Status.AuthorizationMigration.Phase == MigrationPhaseDraining
	if !migrationIsDraining &&
		listener.Status.AppliedPlacement != nil &&
		listener.Status.AppliedPlacement.Fingerprint != "" &&
		listener.Status.AppliedPlacement.Fingerprint != fingerprint &&
		listener.Status.Draining == nil {
		if err := h.startDrain(ctx, listener, "fingerprint changed", listener.Status.AppliedPlacement.Fingerprint); err != nil {
			return errors.Wrap(err, "failed to start drain")
		}
		return nil // persist drain checkpoint; continueDrain runs on next reconcile
	}

	// Step 5.10: Owned children the current intent did not produce (unlabelled
	// prior-policy children, or another fingerprint step 5.9 did not catch) are
	// drained through the persisted checkpoint like every other capture stop;
	// return so it is persisted before continueDrain deletes anything. Skipped
	// while the migration drains, as step 5.9. No drain is active here: step 0
	// returns until it completes.
	if !migrationIsDraining {
		started, staleErr := h.drainStaleChildren(ctx, listener, fingerprint)
		if staleErr != nil {
			return errors.Wrap(staleErr, "failed to drain stale children")
		}
		if started {
			return nil // persist drain checkpoint; continueDrain runs on next reconcile
		}
	}

	// Step 6: Evaluate dual-gate approval (provider + consumer).
	dual, err := h.ensureApprovals(ctx, listener, observerApp, consumerApp, providerApp, &intent)
	if err != nil && dual == nil {
		return errors.Wrap(err, "failed to ensure approvals")
	}

	// Step 6.5: Advance migration for non-fresh installs that have legacy
	// Approvals. The migration state machine needs the dual-gate result to
	// decide whether scoped approvals have converged.
	if listener.Status.AuthorizationPolicyVersion != authorizationPolicyV2 {
		complete, migErr := h.advanceMigration(ctx, listener, &intent, dual)
		if migErr != nil {
			return errors.Wrap(migErr, "migration failed")
		}
		if !complete {
			return nil // requeue; migration in progress
		}
	}

	// Step 7: Handle approval states explicitly.
	switch dual.outcome {
	case outcomeGranted:
		// Continue to provisioning below.

	case outcomeDenied, outcomeRequestDenied:
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
		// applied capture may keep running on it. Transient build and read errors
		// leave capture untouched.
		if gate := scopedIdentityGates(dual); gate != "" {
			return h.stopCaptureAfter(ctx, listener, fmt.Sprintf("approval identity invalid (%s gate)", gate), evalErr)
		}
		return evalErr

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

// checkEarlyRestriction reads the Approvals referenced by the Listener's
// persisted status and returns the gate of one that is conclusively revoked
// (Rejected, Suspended, or Expired-from-Suspended). While capture is applied it
// also reads the referenced current ApprovalRequests and returns the gate of a
// Rejected one; a ref with a UID must match the live request, so NotFound or a
// recreated request is not a rejection. This is a READ-ONLY check that works
// even when the Application/Route topology is temporarily unreachable. Every
// read is attempted; the first read error is returned with any denial found.
func (h *ListenerHandler) checkEarlyRestriction(
	ctx context.Context,
	listener *spectrev1.Listener,
) (approvalGate, requestGate string, err error) {
	c := cclient.ClientFromContextOrDie(ctx)

	var firstErr error
	get := func(ref *ctypes.ObjectRef, obj client.Object) bool {
		if err := c.Get(ctx, ref.K8s(), obj); err != nil {
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
// it: an applied fingerprint, or status refs of capture children (Listeners of
// the prior policy have no applied placement).
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
