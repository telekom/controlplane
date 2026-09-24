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
	owned, err := listOwnedChildren(ctx, listener)
	if err != nil {
		return err
	}
	recordDrain(ctx, listener, reason, oldFingerprint, owned)
	return nil
}

// drainCapture stops this Listener's capture through the persisted drain and
// reports whether a drain is active; the caller must then return (nil from
// CreateOrUpdate, errDrainPending from Delete) so the checkpoint is persisted
// before continueDrain deletes anything. It is the only way revocation, request
// denial, provider invalidation, unsupported placement and Listener deletion
// remove children. The inventory covers status refs and
// owner-labelled children, so partial provisioning is drained too. With nothing
// tracked, found or applied it starts no drain (that drain would complete empty
// and restart on every reconcile), clears a drained placement and touches no
// Publisher.
func (h *ListenerHandler) drainCapture(ctx context.Context, listener *spectrev1.Listener, reason string) (bool, error) {
	if listener.Status.Draining != nil {
		return true, nil // Step 0 advances the active drain.
	}
	owned, err := listOwnedChildren(ctx, listener)
	if err != nil {
		return false, err
	}
	oldFingerprint := ""
	if ap := listener.Status.AppliedPlacement; ap != nil {
		oldFingerprint = ap.Fingerprint
	}
	tracked := listener.Status.RouteListener != nil || len(listener.Status.EventSubscriptions) > 0
	if !tracked && len(owned.routeListeners) == 0 && len(owned.subscribers) == 0 && oldFingerprint == "" {
		listener.Status.AppliedPlacement = nil
		return false, nil
	}
	recordDrain(ctx, listener, reason, oldFingerprint, owned)
	return true, nil
}

// drainStaleChildren starts the persisted drain when any owned child lacks the
// current authorization fingerprint, unlabelled prior-policy children included,
// and reports whether it did. The checkpoint covers the whole owned inventory
// plus status refs; the caller must return so it is persisted before
// continueDrain deletes anything.
func (h *ListenerHandler) drainStaleChildren(ctx context.Context, listener *spectrev1.Listener, fingerprint string) (bool, error) {
	owned, err := listOwnedChildren(ctx, listener)
	if err != nil {
		return false, err
	}
	stale := false
	for i := range owned.routeListeners {
		stale = stale || isStaleChild(owned.routeListeners[i].Labels, fingerprint)
	}
	for i := range owned.subscribers {
		stale = stale || isStaleChild(owned.subscribers[i].Labels, fingerprint)
	}
	if !stale {
		return false, nil
	}
	recordDrain(ctx, listener, "stale authorization generation", "", owned)
	return true, nil
}

// ownedChildren is one owner-label inventory of a Listener's capture children.
type ownedChildren struct {
	routeListeners []gatewayv1.RouteListener
	subscribers    []pubsubv1.Subscriber
}

// listOwnedChildren lists the RouteListeners and Subscribers carrying the
// Listener's owner label.
func listOwnedChildren(ctx context.Context, listener *spectrev1.Listener) (*ownedChildren, error) {
	c := cclient.ClientFromContextOrDie(ctx)
	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return nil, errors.Wrap(err, "failed to list owned RouteListeners for drain snapshot")
	}
	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return nil, errors.Wrap(err, "failed to list owned Subscribers for drain snapshot")
	}
	return &ownedChildren{routeListeners: rlList.Items, subscribers: subList.Items}, nil
}

// recordDrain assigns the drain checkpoint for the old generation.
func recordDrain(
	ctx context.Context,
	listener *spectrev1.Listener,
	reason string,
	oldFingerprint string,
	owned *ownedChildren,
) {
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
	for i := range owned.routeListeners {
		oldRLs = addOldRef(oldRLs, *ctypes.ObjectRefFromObject(&owned.routeListeners[i]))
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
	for i := range owned.subscribers {
		oldSubs = addOldRef(oldSubs, *ctypes.ObjectRefFromObject(&owned.subscribers[i]))
	}
	slices.SortFunc(oldSubs, compareRefs)
	drain.OldSubscribers = oldSubs

	// Snapshot source publisher and event store from applied placement. Without
	// one (partial provisioning, migration of legacy capture) the owned bridges
	// name their Publisher.
	if ap := listener.Status.AppliedPlacement; ap != nil {
		if ap.Publisher != nil {
			drain.SourcePublisher = ap.Publisher.DeepCopy()
		}
		if ap.CaptureEventStore != nil {
			drain.SourceEventStore = ap.CaptureEventStore.DeepCopy()
		}
	}
	if drain.SourcePublisher == nil {
		drain.SourcePublisher = agreedPublisher(owned.subscribers)
	}

	listener.Status.Draining = drain
	log.FromContext(ctx).Info("Started drain", "reason", reason, "oldFingerprint", oldFingerprint)
}

// agreedPublisher returns the Publisher every Subscriber references, or nil
// when there is none or they disagree. CleaningPublisher still covers every old
// Subscriber namespace.
func agreedPublisher(subs []pubsubv1.Subscriber) *ctypes.ObjectRef {
	var ref *ctypes.ObjectRef
	for i := range subs {
		p := &subs[i].Spec.Publisher
		if p.Name == "" || p.Namespace == "" {
			return nil
		}
		if ref != nil && (ref.Namespace != p.Namespace || ref.Name != p.Name) {
			return nil
		}
		ref = p
	}
	if ref == nil {
		return nil
	}
	return ref.DeepCopy()
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
		// Clean up the generic Publisher in every old source namespace once no
		// other Subscriber references it. Advance only when every one succeeded.
		var errs error
		for _, ns := range drainPublisherNamespaces(drain) {
			if err := h.cleanupGenericPublisherIfOrphaned(ctx, ns); err != nil {
				errs = stderrors.Join(errs, errors.Wrapf(err, "failed to cleanup Publisher in namespace %q during drain", ns))
			}
		}
		if errs != nil {
			return false, errs
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

// drainPublisherNamespaces returns the sorted namespaces that may hold the old
// generation's generic Publisher: the recorded source Publisher's and every old
// bridge Subscriber's, since bridges are co-located with their Publisher.
func drainPublisherNamespaces(drain *spectrev1.ListenerDrainStatus) []string {
	var nss []string
	if drain.SourcePublisher != nil {
		nss = append(nss, drain.SourcePublisher.Namespace)
	}
	for i := range drain.OldSubscribers {
		nss = append(nss, drain.OldSubscribers[i].Namespace)
	}
	nss = slices.DeleteFunc(nss, func(ns string) bool { return ns == "" })
	slices.Sort(nss)
	return slices.Compact(nss)
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
