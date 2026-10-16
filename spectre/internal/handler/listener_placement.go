// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
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

// basePathMismatchError reports that a Listener's apiBasePath resolves to the
// provider's Route but is not spelled like the exposure's basePath. It stops
// candidate evaluation and drains applied capture.
type basePathMismatchError struct{ msg string }

func (e *basePathMismatchError) Error() string { return e.msg }

// IsBlocked implements ctrlerrors.BlockedError.
func (e *basePathMismatchError) IsBlocked() bool { return true }

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
// unsuitable for capture, a definitive *util.DeliveryPathError when the zone
// supports capture but cannot deliver to A, or another error when the zone
// cannot be judged (read errors, NotReady dependencies, unverifiable provider
// identity). Only a reason permits trying the next candidate.
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
	// The Route is found by its normalized name, so a path that differs only in
	// case, separators or leading/trailing slashes resolves to it. The
	// RouteListener issue and the bridge filter carry the Listener's path, and
	// the gateway accepts one listener entry per consumer on a Route, so capture
	// is only provisioned for the exposure's exact path.
	if apiBasePath != binding.ApiBasePath {
		return nil, nil, "", &basePathMismatchError{msg: fmt.Sprintf(
			"Listener apiBasePath %q does not match the exposed basePath %q. Use the exact basePath of the API",
			apiBasePath, binding.ApiBasePath)}
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
// capture candidates in order (C's zone, P's zone). Only definitive
// unsuitability for capture moves on to the next candidate; any error stops
// evaluation so capture is never relocated because of it. When no candidate
// works, or a candidate cannot deliver to A, it returns a
// *util.NoCaptureCandidateError naming each zone tried. When A's zone or the
// chosen capture zone publishes events with another environment than its
// EventStore subscribes with, it returns a *util.EnvironmentMismatchError;
// capture is not moved to P's zone, so the misconfiguration stays visible. The
// "default" exemption only adds an EnvironmentAlias to the placement.
func (h *ListenerHandler) resolveListenerPlacement(
	ctx context.Context,
	observerZone, consumerZone, providerZone *adminv1.Zone,
	providerApp *applicationv1.Application,
	apiBasePath string,
) (*util.ListenerPlacement, *ProviderBinding, error) {
	logger := log.FromContext(ctx)

	d, err := util.ResolveDelivery(ctx, observerZone)
	var envMismatch *util.EnvironmentMismatchError
	if errors.As(err, &envMismatch) {
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, errors.Wrap(err, "placement: failed to resolve delivery for observer zone")
	}
	if d.EventConfig.IsProxy() {
		if _, _, err := resolveSSEBackendZone(ctx, observerZone, d.EventConfig); err != nil {
			return nil, nil, errors.Wrap(err, "failed to validate delivery proxy target")
		}
	}

	var rejections []util.CandidateRejection
	for _, zone := range util.CaptureCandidateZones(consumerZone, providerZone) {
		lp, binding, reason, err := h.evaluateCaptureCandidate(ctx, zone, consumerZone, providerApp, apiBasePath, d)
		var noPath *util.DeliveryPathError
		if errors.As(err, &noPath) {
			return nil, nil, &util.NoCaptureCandidateError{ApiBasePath: apiBasePath, Rejections: rejections, DeliveryPath: noPath}
		}
		if err != nil {
			return nil, nil, errors.Wrapf(err, "capture candidate zone %q", zone.Name)
		}
		if reason == "" {
			if err := h.checkCaptureEnvironment(ctx, lp); err != nil {
				return nil, nil, err
			}
			return lp, binding, nil
		}
		logger.V(1).Info("Skipping capture candidate zone", "zone", zone.Name, "reason", reason)
		rejections = append(rejections, util.CandidateRejection{Zone: zone.Name, Reason: reason})
	}
	return nil, nil, &util.NoCaptureCandidateError{ApiBasePath: apiBasePath, Rejections: rejections}
}

// checkCaptureEnvironment compares the environment the capture zone's Jumper
// publishes bridge events with, taken from the same gateway client issuer the
// RouteListener carries, with the capture EventStore's, and adds an
// EnvironmentAlias to lp for the "default" exemption. A missing Realm or
// IssuerUrl is wrapped and returned: it is transient and must not drain capture.
func (h *ListenerHandler) checkCaptureEnvironment(ctx context.Context, lp *util.ListenerPlacement) error {
	_, issuer, err := h.resolveGatewayCredentials(ctx, lp.CaptureZone)
	if err != nil {
		return errors.Wrapf(err, "capture candidate zone %q", lp.CaptureZone.Name)
	}
	alias, err := util.CheckCaptureEnvironment(ctx, lp.CaptureZone, issuer, lp.CaptureEventConfig, lp.CaptureEventStore)
	if alias != nil {
		lp.EnvironmentAliases = append(lp.EnvironmentAliases, alias)
	}
	return err
}

// warnEnvironmentAliases reports each EnvironmentAlias on obj as a Warning
// event, or logs it when the context carries no recorder.
func warnEnvironmentAliases(ctx context.Context, obj runtime.Object, aliases ...*util.EnvironmentAlias) {
	recorder, hasRecorder := contextutil.RecoderFromContext(ctx)
	for _, a := range aliases {
		if a == nil {
			continue
		}
		if hasRecorder {
			recorder.Event(obj, "Warning", "EnvironmentAliasAssumed", a.String())
			continue
		}
		log.FromContext(ctx).Info("Event environment relies on a Horizon defaultEnvironment alias",
			"role", a.Role, "zone", a.Zone, "eventStore", a.EventStore,
			"publishEnvironment", a.PublishEnvironment, "subscribeEnvironment", "default")
	}
}

// handlePlacementError stops capture that can no longer be placed and returns
// the error to report. When no candidate supports capture, the capture zone
// cannot deliver to A, a zone publishes events with another environment than
// its EventStore subscribes with, the apiBasePath is not spelled like the
// exposure's, or the provider binding is definitively invalidated, capture is
// drained (checkpoint first), including untracked children of a partial
// provisioning. Other errors, including read errors, leave capture untouched:
// a transient failure must not stop or move it.
func (h *ListenerHandler) handlePlacementError(ctx context.Context, listener *spectrev1.Listener, err error) error {
	var noCandidate *util.NoCaptureCandidateError
	var bindingErr *providerBindingError
	var envMismatch *util.EnvironmentMismatchError
	var pathMismatch *basePathMismatchError
	reason, notStarted := "", err
	switch {
	case errors.As(err, &noCandidate) && noCandidate.DeliveryPath != nil:
		reason = "capture zone cannot deliver to observer"
	case errors.As(err, &noCandidate):
		reason = "no supported capture placement"
	case errors.As(err, &envMismatch):
		reason = "event environment mismatch"
	case errors.As(err, &pathMismatch):
		reason = "apiBasePath mismatch"
	case errors.As(err, &bindingErr):
		reason, notStarted = "provider binding invalidated", errors.Wrap(err, "provider binding check failed")
	default:
		return errors.Wrap(err, "failed to resolve placement")
	}

	started, drainErr := h.drainCapture(ctx, listener, reason)
	if drainErr != nil {
		return errors.Wrapf(drainErr, "failed to start drain (%s)", reason)
	}
	listener.SetCondition(condition.NewNotReadyCondition(condition.ReasonPreconditionNotMet, err.Error()))
	listener.SetCondition(condition.NewBlockedCondition(err.Error()))
	if !started {
		return notStarted
	}
	return nil // persist drain checkpoint
}
