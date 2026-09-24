// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler/util"
)

// providerBindingError marks a definitive provider-binding invalidation (a
// BlockedError other than an off-path Route): missing owner label, exposure
// not found or inactive, missing application label, provider mismatch. It
// stops candidate evaluation and drains applied capture. Read errors are never
// wrapped in it: they must leave capture untouched.
type providerBindingError struct{ err error }

func (e *providerBindingError) Error() string { return e.err.Error() }
func (e *providerBindingError) Unwrap() error { return e.err }

// onConsumerPath reports whether a provider-bound Route in zone carries C's
// traffic to P. In C's zone, C calls the Route the exposure assigned to that
// zone (real or proxy). Outside C's zone only the exposure's primary route
// qualifies, and only when the exposure proxies C's zone to it; a proxy route
// in any other zone serves other subscribers.
func onConsumerPath(zone, consumerZone *adminv1.Zone, b *ProviderBinding) bool {
	if util.SameZone(zone, consumerZone) {
		return true
	}
	if !b.IsPrimaryRoute {
		return false
	}
	for i := range b.ProxyRoutes {
		if b.ProxyRoutes[i].Namespace == consumerZone.Status.Namespace {
			return true
		}
	}
	return false
}

// evaluateCaptureCandidate checks one capture candidate zone. It returns the
// placement and binding on success, a reason when the zone is definitively
// unsuitable, or an error when the zone cannot be judged (read errors, NotReady
// dependencies, unverifiable provider identity). Only a reason permits trying
// the next candidate.
func (h *ListenerHandler) evaluateCaptureCandidate(
	ctx context.Context,
	zone, consumerZone *adminv1.Zone,
	providerApp *applicationv1.Application,
	apiBasePath string,
	d *util.DeliveryPlacement,
) (*util.ListenerPlacement, *ProviderBinding, string, error) {
	route, err := h.findRouteByPath(ctx, zone.Status.Namespace, apiBasePath)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "failed to find Route for apiBasePath")
	}
	if route == nil {
		return nil, nil, fmt.Sprintf("no Route %q in namespace %q", util.MakeRouteName(apiBasePath), zone.Status.Namespace), nil
	}

	// Pass-through routes skip authentication entirely (the route handler wraps
	// RouteListener collection in `if !route.Spec.PassThrough`), and failover
	// routes overwrite the /listener upstream to /proxy (priority 109 > 103).
	if route.Spec.PassThrough {
		return nil, nil, fmt.Sprintf("Route %q is pass-through — listener capture is not supported for this route mode", route.Name), nil
	}
	if route.Spec.Traffic.Failover != nil {
		return nil, nil, fmt.Sprintf("Route %q is failover — listener capture is not supported for this route mode", route.Name), nil
	}

	binding, err := h.verifyProviderBinding(ctx, route, providerApp)
	if err != nil {
		var offPath *routeOffPathError
		if errors.As(err, &offPath) {
			return nil, nil, offPath.Error(), nil
		}
		var blocked ctrlerrors.BlockedError
		if errors.As(err, &blocked) && blocked.IsBlocked() {
			return nil, nil, "", &providerBindingError{err: err}
		}
		return nil, nil, "", errors.Wrap(err, "failed to verify provider binding")
	}
	if !onConsumerPath(zone, consumerZone, binding) {
		return nil, nil, fmt.Sprintf("Route %q is referenced by ApiExposure %q for another subscriber zone, not on the consumer→provider path",
			route.Name, binding.ApiExposureName), nil
	}

	lp, reason, err := util.ResolveCaptureZone(ctx, zone, route, d)
	if err != nil || reason != "" {
		return nil, nil, reason, err
	}
	// A proxy capture zone's backend must be a Ready local target zone; chains
	// and cycles block. The delivery zone was validated by the caller.
	if lp.CaptureEventConfig.IsProxy() && !util.SameZone(zone, d.Zone) {
		if _, _, err := resolveSSEBackendZone(ctx, zone, lp.CaptureEventConfig); err != nil {
			return nil, nil, "", errors.Wrap(err, "failed to validate capture proxy target")
		}
	}
	return lp, binding, "", nil
}

// resolveListenerPlacement resolves delivery in A's zone, then evaluates the
// capture candidates in order (A's zone when on the C→P path, C's zone, P's
// zone). Only definitive unsuitability moves on to the next candidate; any
// error stops evaluation so capture is never relocated because of it. When no
// candidate works it returns a *util.NoCaptureCandidateError naming each zone.
func (h *ListenerHandler) resolveListenerPlacement(
	ctx context.Context,
	observerZone, consumerZone, providerZone *adminv1.Zone,
	providerApp *applicationv1.Application,
	apiBasePath string,
) (*util.ListenerPlacement, *ProviderBinding, error) {
	logger := log.FromContext(ctx)

	d, err := util.ResolveDelivery(ctx, observerZone)
	if err != nil {
		return nil, nil, err
	}
	if d.EventConfig.IsProxy() {
		if _, _, err := resolveSSEBackendZone(ctx, observerZone, d.EventConfig); err != nil {
			return nil, nil, errors.Wrap(err, "failed to validate delivery proxy target")
		}
	}

	candidates, rejections := util.CaptureCandidateZones(observerZone, consumerZone, providerZone)
	for _, zone := range candidates {
		lp, binding, reason, err := h.evaluateCaptureCandidate(ctx, zone, consumerZone, providerApp, apiBasePath, d)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "capture candidate zone %q", zone.Name)
		}
		if reason == "" {
			return lp, binding, nil
		}
		logger.V(1).Info("Skipping capture candidate zone", "zone", zone.Name, "reason", reason)
		rejections = append(rejections, util.CandidateRejection{Zone: zone.Name, Reason: reason})
	}
	return nil, nil, &util.NoCaptureCandidateError{ApiBasePath: apiBasePath, Rejections: rejections}
}

// handlePlacementError stops applied capture that can no longer be placed and
// returns the error to report. When no candidate supports capture, applied
// capture is drained (checkpoint first) or stray children are removed directly.
// A definitive provider-binding invalidation also drains applied capture. Other
// errors, including read errors, leave capture untouched: a transient failure
// must not stop or move it.
func (h *ListenerHandler) handlePlacementError(ctx context.Context, listener *spectrev1.Listener, err error) error {
	var noCandidate *util.NoCaptureCandidateError
	if errors.As(err, &noCandidate) {
		started, drainErr := h.drainAppliedCapture(ctx, listener, "no supported capture placement")
		if drainErr != nil {
			return errors.Wrap(drainErr, "failed to start drain after placement loss")
		}
		if started {
			listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, err.Error()))
			listener.SetCondition(condition.NewBlockedCondition(err.Error()))
			return nil // persist drain checkpoint
		}
		if cleanupErr := h.removeUnplacedChildren(ctx, listener); cleanupErr != nil {
			return cleanupErr
		}
		return err
	}

	var bindingErr *providerBindingError
	if errors.As(err, &bindingErr) {
		started, drainErr := h.drainAppliedCapture(ctx, listener, "provider binding invalidated")
		if drainErr != nil {
			return errors.Wrap(drainErr, "failed to start drain after binding failure")
		}
		if started {
			listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, err.Error()))
			listener.SetCondition(condition.NewBlockedCondition(err.Error()))
			return nil // persist drain checkpoint
		}
		return errors.Wrap(err, "provider binding check failed")
	}

	return errors.Wrap(err, "failed to resolve placement")
}

// removeUnplacedChildren directly deletes owner-labelled children of a Listener
// without applied capture, then the generic Publisher if it became orphaned.
func (h *ListenerHandler) removeUnplacedChildren(ctx context.Context, listener *spectrev1.Listener) error {
	if err := h.deleteAllOwnedChildren(ctx, listener); err != nil {
		return errors.Wrap(err, "failed to cleanup children for unsupported capture placement")
	}
	listener.Status.RouteListener = nil
	listener.Status.EventSubscriptions = nil

	zoneNamespace, err := h.resolvePublisherNamespace(ctx, listener)
	if err != nil {
		return errors.Wrap(err, "failed to resolve publisher namespace for unsupported capture placement")
	}
	if zoneNamespace != "" {
		if err := h.cleanupGenericPublisherIfOrphaned(ctx, zoneNamespace); err != nil {
			return errors.Wrap(err, "failed to check orphaned generic Publisher")
		}
	}
	return nil
}
