// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

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

	// Snapshot old children from status refs.
	if listener.Status.RouteListener != nil {
		drain.OldRouteListener = listener.Status.RouteListener.DeepCopy()
	}
	if len(listener.Status.EventSubscriptions) > 0 {
		drain.OldSubscribers = make([]ctypes.ObjectRef, len(listener.Status.EventSubscriptions))
		for i := range listener.Status.EventSubscriptions {
			drain.OldSubscribers[i] = *listener.Status.EventSubscriptions[i].DeepCopy()
		}
	}

	// Recover partial provisioning: discover owner-labelled children that might
	// not appear in status refs (e.g. status update failed after creation).
	rlList := &gatewayv1.RouteListenerList{}
	if err := c.List(ctx, rlList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned RouteListeners for drain snapshot")
	}
	for i := range rlList.Items {
		rl := &rlList.Items[i]
		if drain.OldRouteListener == nil || (drain.OldRouteListener.Name != rl.Name || drain.OldRouteListener.Namespace != rl.Namespace) {
			ref := ctypes.ObjectRefFromObject(rl)
			drain.OldRouteListener = ref
		} else if drain.OldRouteListener.UID == "" {
			// Enrich the status ref with the live UID for safe deletion.
			drain.OldRouteListener.UID = rl.UID
		}
	}

	subList := &pubsubv1.SubscriberList{}
	if err := c.List(ctx, subList, cclient.OwnedByLabel(listener)...); err != nil {
		return errors.Wrap(err, "failed to list owned Subscribers for drain snapshot")
	}
	known := make(map[string]struct{}, len(drain.OldSubscribers))
	for i := range drain.OldSubscribers {
		key := drain.OldSubscribers[i].Namespace + "/" + drain.OldSubscribers[i].Name
		known[key] = struct{}{}
	}
	for i := range subList.Items {
		sub := &subList.Items[i]
		key := sub.Namespace + "/" + sub.Name
		if _, exists := known[key]; !exists {
			drain.OldSubscribers = append(drain.OldSubscribers, *ctypes.ObjectRefFromObject(sub))
		} else {
			// Enrich existing refs with the live UID for safe deletion.
			for j := range drain.OldSubscribers {
				if drain.OldSubscribers[j].Name == sub.Name && drain.OldSubscribers[j].Namespace == sub.Namespace && drain.OldSubscribers[j].UID == "" {
					drain.OldSubscribers[j].UID = sub.UID
				}
			}
		}
	}

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

	c := cclient.ClientFromContextOrDie(ctx)

	switch drain.Phase {
	case DrainPhaseStopping:
		// Delete old RouteListener, then verify it is gone.
		if drain.OldRouteListener != nil {
			rl := &gatewayv1.RouteListener{}
			err := c.Get(ctx, drain.OldRouteListener.K8s(), rl)
			if err == nil {
				// UID check: if the live object has a different UID than what we
				// recorded, someone recreated it — treat the old one as gone.
				if drain.OldRouteListener.UID != "" && rl.UID != drain.OldRouteListener.UID {
					logger.Info("Old RouteListener UID differs, treating as gone", "name", drain.OldRouteListener.Name)
				} else {
					// Delete with UID+RV preconditions to guard against a replaced child.
					uid := rl.UID
					rv := rl.ResourceVersion
					precond := client.Preconditions(metav1.Preconditions{
						UID:             &uid,
						ResourceVersion: &rv,
					})
					if delErr := c.Delete(ctx, rl, precond); delErr != nil {
						if apierrors.IsConflict(delErr) {
							// Object changed — re-evaluate next reconcile.
							return false, nil
						}
						if !apierrors.IsNotFound(delErr) {
							return false, errors.Wrapf(delErr, "failed to delete old RouteListener %q", drain.OldRouteListener.Name)
						}
					}
					logger.Info("Deleted old RouteListener during drain", "name", drain.OldRouteListener.Name)
					return false, nil // Requeue to verify deletion
				}
			} else if !apierrors.IsNotFound(err) {
				return false, errors.Wrapf(err, "failed to check old RouteListener %q", drain.OldRouteListener.Name)
			}
		}
		// Clear the status ref — the old RouteListener is gone.
		if listener.Status.RouteListener != nil &&
			drain.OldRouteListener != nil &&
			listener.Status.RouteListener.Name == drain.OldRouteListener.Name &&
			listener.Status.RouteListener.Namespace == drain.OldRouteListener.Namespace {
			listener.Status.RouteListener = nil
		}
		drain.Phase = DrainPhaseDrainingSubscribers
		logger.Info("Drain: old RouteListener gone, advancing to DrainingSubscribers")
		return false, nil // Persist phase advancement

	case DrainPhaseDrainingSubscribers:
		// Delete each old Subscriber, then verify all are gone.
		allGone := true
		for i := range drain.OldSubscribers {
			ref := &drain.OldSubscribers[i]
			sub := &pubsubv1.Subscriber{}
			err := c.Get(ctx, ref.K8s(), sub)
			if err == nil {
				// UID check.
				if ref.UID != "" && sub.UID != ref.UID {
					logger.V(1).Info("Old Subscriber UID differs, treating as gone", "name", ref.Name)
					continue
				}
				// Delete with UID+RV preconditions to guard against a replaced child.
				uid := sub.UID
				rv := sub.ResourceVersion
				precond := client.Preconditions(metav1.Preconditions{
					UID:             &uid,
					ResourceVersion: &rv,
				})
				if delErr := c.Delete(ctx, sub, precond); delErr != nil {
					if apierrors.IsConflict(delErr) {
						// Object changed — re-evaluate next reconcile.
						allGone = false
						continue
					}
					if !apierrors.IsNotFound(delErr) {
						return false, errors.Wrapf(delErr, "failed to delete old Subscriber %q", ref.Name)
					}
				}
				logger.Info("Deleted old Subscriber during drain", "name", ref.Name)
				allGone = false
			} else if !apierrors.IsNotFound(err) {
				return false, errors.Wrapf(err, "failed to check old Subscriber %q", ref.Name)
			}
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
