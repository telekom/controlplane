// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// startDrain records the current applied state and marks the Listener as draining.
// It snapshots the old children so that continueDrain can verify they have been
// removed after removeStaleChildren runs.
func (h *ListenerHandler) startDrain(
	ctx context.Context,
	listener *spectrev1.Listener,
	reason string,
	oldFingerprint string,
) error {
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

	listener.Status.Draining = drain
	logger.Info("Started drain", "reason", reason, "oldFingerprint", oldFingerprint)
	return nil
}

// continueDrain checks if the old generation has been fully cleaned up.
// Returns true when drain is complete and the Listener can proceed to
// provisioning with the new intent.
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
		// Verify old RouteListener is gone.
		if drain.OldRouteListener != nil {
			rl := &gatewayv1.RouteListener{}
			err := c.Get(ctx, drain.OldRouteListener.K8s(), rl)
			if err == nil {
				// Still exists — removeStaleChildren will delete it.
				logger.V(1).Info("Old RouteListener still exists, waiting", "name", drain.OldRouteListener.Name)
				return false, nil
			}
			if !apierrors.IsNotFound(err) {
				return false, errors.Wrapf(err, "failed to check old RouteListener %q", drain.OldRouteListener.Name)
			}
		}
		drain.Phase = DrainPhaseDrainingSubscribers
		logger.Info("Drain: old RouteListener gone, advancing to DrainingSubscribers")
		fallthrough

	case DrainPhaseDrainingSubscribers:
		// Verify all old Subscribers are gone.
		for i := range drain.OldSubscribers {
			ref := &drain.OldSubscribers[i]
			sub := &pubsubv1.Subscriber{}
			err := c.Get(ctx, ref.K8s(), sub)
			if err == nil {
				logger.V(1).Info("Old Subscriber still exists, waiting", "name", ref.Name)
				return false, nil
			}
			if !apierrors.IsNotFound(err) {
				return false, errors.Wrapf(err, "failed to check old Subscriber %q", ref.Name)
			}
		}
		logger.Info("Drain: all old children removed, drain complete")
		listener.Status.Draining = nil
		return true, nil

	case DrainPhaseCleaningPublisher:
		// Stub — full Publisher cleanup is E2E territory.
		listener.Status.Draining = nil
		return true, nil

	case DrainPhaseComplete:
		listener.Status.Draining = nil
		return true, nil

	default:
		return false, errors.Errorf("unknown drain phase %q", drain.Phase)
	}
}
