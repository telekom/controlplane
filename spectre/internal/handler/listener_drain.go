// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"cmp"
	"context"
	stderrors "errors"
	"slices"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// Drain phase constants match the ListenerDrainStatus.Phase enum.
const (
	DrainPhaseStopping            = "Stopping"
	DrainPhaseDrainingSubscribers = "DrainingSubscribers"
	DrainPhaseCleaningPublisher   = "CleaningPublisher"
	DrainPhaseComplete            = "Complete"
)

// startDrain snapshots the old generation and marks the Listener as draining.
// The caller MUST return nil immediately after startDrain so that the controller
// persists the drain checkpoint before any destructive work begins.
// continueDrain performs the actual deletions on a subsequent reconcile.
func (h *ListenerHandler) startDrain(
	ctx context.Context,
	listener *spectrev1.Listener,
	reason string,
	oldFingerprint string,
) error {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	drain := &spectrev1.ListenerDrainStatus{
		Phase:          DrainPhaseStopping,
		Reason:         reason,
		OldFingerprint: oldFingerprint,
	}

	// Snapshot old children from status refs, then recover partial provisioning:
	// owner-labelled children that might not appear in status refs (e.g. status
	// update failed after creation, or a child left in another zone). Live
	// objects fill in missing UIDs for safe deletion. Both inventories are sorted
	// because cached List order is not stable.
	var oldRLs []ctypes.ObjectRef
	if listener.Status.RouteListener != nil {
		oldRLs = addOldRef(oldRLs, *listener.Status.RouteListener)
	}
	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned RouteListeners for drain snapshot")
	}
	for i := range rlList.Items {
		oldRLs = addOldRef(oldRLs, *ctypes.ObjectRefFromObject(&rlList.Items[i]))
	}
	slices.SortFunc(oldRLs, compareRefs)
	drain.OldRouteListeners = oldRLs
	if len(oldRLs) > 0 {
		// Mirror for controller builds that only read the singular field.
		drain.OldRouteListener = oldRLs[0].DeepCopy()
	}

	var oldSubs []ctypes.ObjectRef
	for i := range listener.Status.EventSubscriptions {
		oldSubs = addOldRef(oldSubs, listener.Status.EventSubscriptions[i])
	}
	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned Subscribers for drain snapshot")
	}
	for i := range subList.Items {
		oldSubs = addOldRef(oldSubs, *ctypes.ObjectRefFromObject(&subList.Items[i]))
	}
	slices.SortFunc(oldSubs, compareRefs)
	drain.OldSubscribers = oldSubs

	// Snapshot source publisher and event store from applied placement.
	if ap := listener.Status.AppliedPlacement; ap != nil {
		if ap.Publisher != nil {
			drain.SourcePublisher = ap.Publisher.DeepCopy()
		}
		if ap.CaptureEventStore != nil {
			drain.SourceEventStore = ap.CaptureEventStore.DeepCopy()
		}
	}

	listener.Status.Draining = drain
	logger.Info("Started drain", "reason", reason, "oldFingerprint", oldFingerprint)
	return nil
}

// drainAppliedCapture starts a persisted drain of the applied capture and
// reports whether it did; the caller must then return so the checkpoint is
// persisted before continueDrain deletes anything. It never starts a drain
// without tracked children: that drain would complete empty and restart on
// every reconcile. A drained placement (no children, empty fingerprint) is
// cleared instead.
func (h *ListenerHandler) drainAppliedCapture(ctx context.Context, listener *spectrev1.Listener, reason string) (bool, error) {
	ap := listener.Status.AppliedPlacement
	if ap == nil || listener.Status.Draining != nil {
		return false, nil
	}
	if listener.Status.RouteListener == nil && len(listener.Status.EventSubscriptions) == 0 {
		if ap.Fingerprint == "" {
			listener.Status.AppliedPlacement = nil
		}
		return false, nil
	}
	return true, h.startDrain(ctx, listener, reason, ap.Fingerprint)
}

// continueDrain advances the drain state machine. It performs deletions with
// UID checks and returns true when the drain is complete.
func (h *ListenerHandler) continueDrain(
	ctx context.Context,
	listener *spectrev1.Listener,
) (bool, error) {
	logger := log.FromContext(ctx)
	drain := listener.Status.Draining
	if drain == nil {
		return true, nil
	}

	switch drain.Phase {
	case DrainPhaseStopping:
		// Delete every old RouteListener, then verify all are gone before any
		// Subscriber is touched.
		allGone, err := deleteRecorded(ctx, drainRouteListenerRefs(listener),
			func() client.Object { return &gatewayv1.RouteListener{} })
		if err != nil {
			return false, errors.Wrap(err, "failed to delete old RouteListeners")
		}
		if !allGone {
			return false, nil // Requeue to verify deletion
		}
		// Clear the status ref — it is part of the drained set, so it is gone.
		listener.Status.RouteListener = nil
		drain.Phase = DrainPhaseDrainingSubscribers
		logger.Info("Drain: old RouteListeners gone, advancing to DrainingSubscribers")
		return false, nil // Persist phase advancement

	case DrainPhaseDrainingSubscribers:
		// Delete each old Subscriber, then verify all are gone.
		allGone, err := deleteRecorded(ctx, drain.OldSubscribers,
			func() client.Object { return &pubsubv1.Subscriber{} })
		if err != nil {
			return false, errors.Wrap(err, "failed to delete old Subscribers")
		}
		if !allGone {
			return false, nil // Requeue to verify finalization
		}
		// Clear status refs that matched drained subscribers.
		listener.Status.EventSubscriptions = nil
		drain.Phase = DrainPhaseCleaningPublisher
		logger.Info("Drain: all old Subscribers gone, advancing to CleaningPublisher")
		return false, nil // Persist phase advancement

	case DrainPhaseCleaningPublisher:
		// Clean up the generic Publisher if no other Subscribers reference it.
		if drain.SourcePublisher != nil {
			ns := drain.SourcePublisher.Namespace
			if ns != "" {
				if err := h.cleanupGenericPublisherIfOrphaned(ctx, ns); err != nil {
					return false, errors.Wrap(err, "failed to cleanup Publisher during drain")
				}
			}
		}
		logger.Info("Drain complete")
		listener.Status.Draining = nil
		return true, nil

	case DrainPhaseComplete:
		listener.Status.Draining = nil
		return true, nil

	default:
		return false, errors.Errorf("unknown drain phase %q", drain.Phase)
	}
}

// addOldRef adds ref to an old-generation inventory keyed by namespace/name.
// A missing UID is filled from the other record; two different UIDs for one
// name are distinct instances and both stay recorded.
func addOldRef(refs []ctypes.ObjectRef, ref ctypes.ObjectRef) []ctypes.ObjectRef {
	for i := range refs {
		r := &refs[i]
		if r.Namespace != ref.Namespace || r.Name != ref.Name {
			continue
		}
		if r.UID == "" {
			r.UID = ref.UID
			return refs
		}
		if ref.UID == "" || ref.UID == r.UID {
			return refs
		}
	}
	return append(refs, ref)
}

// compareRefs orders ObjectRefs by namespace, name, then UID.
func compareRefs(a, b ctypes.ObjectRef) int {
	return cmp.Or(
		cmp.Compare(a.Namespace, b.Namespace),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.UID, b.UID),
	)
}

// drainRouteListenerRefs returns every RouteListener the Stopping phase must
// remove: the recorded set, the singular record of checkpoints written by
// earlier builds, and the status ref those builds could drop from the
// checkpoint. Provisioning is blocked while a drain is active, so the status
// ref is always old-generation here.
func drainRouteListenerRefs(listener *spectrev1.Listener) []ctypes.ObjectRef {
	drain := listener.Status.Draining
	refs := slices.Clone(drain.OldRouteListeners)
	for _, ref := range []*ctypes.ObjectRef{drain.OldRouteListener, listener.Status.RouteListener} {
		if ref != nil {
			refs = addOldRef(refs, *ref)
		}
	}
	return refs
}

// deleteRecorded requests deletion of each recorded instance that still exists
// and reports whether all of them are gone. NotFound or a different live UID
// counts as gone: the same-name replacement is never deleted. Every ref is
// attempted; read and delete errors are joined and returned.
func deleteRecorded(ctx context.Context, refs []ctypes.ObjectRef, newObj func() client.Object) (bool, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	logger := log.FromContext(ctx)

	allGone := true
	var errs error
	for _, ref := range refs {
		// A fresh object per Get: decoding into a reused one can keep stale fields.
		obj := newObj()
		if err := c.Get(ctx, ref.K8s(), obj); err != nil {
			if !apierrors.IsNotFound(err) {
				allGone = false
				errs = stderrors.Join(errs, errors.Wrapf(err, "failed to check %s", ref.String()))
			}
			continue
		}
		// UID check: a different live UID means the recorded instance is gone
		// and someone recreated the name.
		if ref.UID != "" && obj.GetUID() != ref.UID {
			logger.Info("Recorded drain instance replaced, treating as gone", "ref", ref.String())
			continue
		}
		allGone = false
		// Delete with UID+RV preconditions to guard against a replaced child.
		uid := obj.GetUID()
		rv := obj.GetResourceVersion()
		precond := client.Preconditions(metav1.Preconditions{UID: &uid, ResourceVersion: &rv})
		if err := c.Delete(ctx, obj, precond); err != nil {
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
				// Object changed or vanished — re-evaluate next reconcile.
				continue
			}
			errs = stderrors.Join(errs, errors.Wrapf(err, "failed to delete %s", ref.String()))
			continue
		}
		logger.Info("Requested deletion during drain", "ref", ref.String())
	}
	return allGone, errs
}
