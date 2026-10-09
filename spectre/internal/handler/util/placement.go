// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

// ListenerPlacement holds the fully resolved placement resources for a Listener.
// The handler projects relevant fields from this into the compact PlacementIntent
// (defined in the handler package) for authorization fingerprinting.
type ListenerPlacement struct {
	CaptureZone        *adminv1.Zone
	CaptureRoute       *gatewayv1.Route
	CaptureEventConfig *eventv1.EventConfig
	CaptureEventStore  *pubsubv1.EventStore

	// CallbackOriginZone is the zone whose EventConfig advertises the bridge
	// callback ingress: the capture zone (see CallbackOriginZone).
	CallbackOriginZone *adminv1.Zone

	DeliveryZone        *adminv1.Zone
	DeliveryEventConfig *eventv1.EventConfig
	DeliveryEventStore  *pubsubv1.EventStore

	// BridgeNamespace is the Kubernetes namespace where bridge Subscribers and
	// the shared generic Publisher live. Derived from CaptureEventStore.Namespace.
	BridgeNamespace string

	// CallbackBaseURL is the external gateway URL used for event delivery
	// callbacks: the delivery EventConfig's Status.CallbackURL when the origin is
	// the delivery zone, otherwise the capture EventConfig's
	// Status.ProxyCallbackURLs[delivery zone]. Either way the final
	// DynamicUpstream hop runs on A's gateway, so localhost:8080 is A's Jumper.
	CallbackBaseURL string
}

// SameZone reports whether a and b are the same Zone (name and namespace).
func SameZone(a, b *adminv1.Zone) bool {
	return a.Name == b.Name && a.Namespace == b.Namespace
}

// CandidateRejection records why a capture candidate zone was skipped.
type CandidateRejection struct {
	Zone   string
	Reason string
}

// NoCaptureCandidateError reports that no candidate zone supports capture, or
// that the first zone supporting capture cannot deliver to the observer (A)
// zone (DeliveryPath set). It is a BlockedError: the Listener waits until the
// topology changes.
type NoCaptureCandidateError struct {
	ApiBasePath string
	Rejections  []CandidateRejection
	// DeliveryPath, when set, is why the zone that supports capture cannot
	// deliver to A. Rejections then lists the zones skipped before it.
	DeliveryPath *DeliveryPathError
}

func (e *NoCaptureCandidateError) Error() string {
	parts := make([]string, 0, len(e.Rejections))
	for _, r := range e.Rejections {
		parts = append(parts, fmt.Sprintf("zone %q: %s", r.Zone, r.Reason))
	}
	if e.DeliveryPath == nil {
		return fmt.Sprintf("no supported capture zone for path %q: %s", e.ApiBasePath, strings.Join(parts, "; "))
	}
	msg := fmt.Sprintf("capture zone %q for path %q cannot deliver to observer zone %q: %s; "+
		"capture is not moved to another zone because Listeners with the same consumer, provider and path capture in one zone",
		e.DeliveryPath.CaptureZone, e.ApiBasePath, e.DeliveryPath.DeliveryZone, e.DeliveryPath.Reason)
	if len(parts) > 0 {
		msg += " (skipped " + strings.Join(parts, "; ") + ")"
	}
	return msg
}

// Unwrap returns the DeliveryPathError, if any.
func (e *NoCaptureCandidateError) Unwrap() error {
	if e.DeliveryPath == nil {
		return nil
	}
	return e.DeliveryPath
}

// IsBlocked implements ctrlerrors.BlockedError.
func (e *NoCaptureCandidateError) IsBlocked() bool { return true }

// DeliveryPathError reports that a capture zone cannot deliver to the
// observer (A) zone: the capture EventConfig's mesh excludes A's zone, it has
// no ProxyCallbackURLs entry for A's zone, or, when capture is in A's zone, the
// delivery EventConfig has no CallbackURL. It is a BlockedError and ends
// candidate evaluation: capture is never moved to another zone to reach A.
type DeliveryPathError struct {
	CaptureZone  string
	DeliveryZone string
	Reason       string
}

func (e *DeliveryPathError) Error() string { return e.Reason }

// IsBlocked implements ctrlerrors.BlockedError.
func (e *DeliveryPathError) IsBlocked() bool { return true }

// CaptureCandidateZones returns the capture candidates in evaluation order:
// the consumer (C) zone, then the provider (P) zone, deduplicated by zone
// identity. Gateway traffic from C to P only transits these two zones.
//
// The order never depends on the observer (A). Listeners that share C, P and
// path must capture in the same zone: the Jumpers of two capture zones would
// both capture the same request, and since the bridge trigger carries no zone,
// each Listener's bridge would accept both captures and deliver every event
// twice.
func CaptureCandidateZones(consumerZone, providerZone *adminv1.Zone) []*adminv1.Zone {
	if SameZone(consumerZone, providerZone) {
		return []*adminv1.Zone{consumerZone}
	}
	return []*adminv1.Zone{consumerZone, providerZone}
}

// DeliveryPlacement is the observer (A) zone's event infrastructure. Delivery
// always happens there; it is never moved to another zone.
type DeliveryPlacement struct {
	Zone        *adminv1.Zone
	EventConfig *eventv1.EventConfig
	EventStore  *pubsubv1.EventStore
}

// ResolveDelivery resolves A's EventConfig and EventStore. Any failure blocks
// the Listener; there is no fallback to the consumer or provider zone.
func ResolveDelivery(ctx context.Context, observerZone *adminv1.Zone) (*DeliveryPlacement, error) {
	ec, err := GetEventConfig(ctx, observerZone)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to get delivery EventConfig for observer zone")
	}
	es, err := ResolveEventStore(ctx, ec)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to resolve delivery EventStore for observer zone")
	}
	return &DeliveryPlacement{Zone: observerZone, EventConfig: ec, EventStore: es}, nil
}

// CallbackOriginZone returns the zone whose EventConfig advertises the bridge
// callback ingress for events captured in captureZone: the capture zone itself.
// The bridge Subscribers live in the capture EventStore, which a proxy zone
// owns as its own CR even though its target backend's Horizon delivers them.
//
// For a local zone S, the event domain builds Status.ProxyCallbackURLs[T] as a
// Route on S's gateway whose upstream is T's gateway /horizon-T/callback/v1,
// T's primary callback Route, whose DynamicUpstream makes the final hop from
// T's gateway. A proxy zone owns no such Routes; its CallbackURL and
// ProxyCallbackURLs are projected from its target backend, so the first hop is
// the backend's gateway. Either way the origin EventConfig's entry for the
// delivery zone ends at A's gateway -> localhost:8080 at A's Jumper. This is
// the same exposure-keyed lookup the event domain uses for EventSubscriptions.
func CallbackOriginZone(captureZone *adminv1.Zone, captureEC *eventv1.EventConfig) (*adminv1.Zone, error) {
	if captureEC.Spec.Zone.Name != captureZone.Name {
		return nil, errors.Errorf("placement: EventConfig %q belongs to zone %q, not capture zone %q",
			captureEC.Name, captureEC.Spec.Zone.Name, captureZone.Name)
	}
	return captureZone, nil
}

// ResolveCaptureZone resolves capture in captureZone for delivery d. It returns
// the placement on success, a non-empty reason when the zone is definitively
// unsuitable for capture (no EventConfig), a *DeliveryPathError when the zone
// cannot deliver to A (mesh excludes the delivery zone, no callback path to A),
// or another error for transient or ambiguous state (read errors, NotReady
// EventConfig or EventStore, duplicate EventConfigs). Callers may try another
// candidate only for a reason, never for an error.
func ResolveCaptureZone(
	ctx context.Context,
	captureZone *adminv1.Zone,
	route *gatewayv1.Route,
	d *DeliveryPlacement,
) (*ListenerPlacement, string, error) {
	ec, es := d.EventConfig, d.EventStore
	if !SameZone(captureZone, d.Zone) {
		var err error
		ec, err = FindEventConfig(ctx, captureZone)
		if err != nil {
			return nil, "", errors.Wrap(err, "placement: failed to get capture EventConfig")
		}
		if ec == nil {
			return nil, fmt.Sprintf("zone %q has no EventConfig", captureZone.Name), nil
		}
		if condition.EnsureReady(ec) != nil {
			return nil, "", ctrlerrors.BlockedErrorf("placement: capture EventConfig %q for zone %q is not ready",
				ec.Name, captureZone.Name)
		}
		es, err = ResolveEventStore(ctx, ec)
		if err != nil {
			return nil, "", errors.Wrap(err, "placement: failed to resolve capture EventStore")
		}
	}

	if !ec.SupportsZone(d.Zone.Name) {
		return nil, "", &DeliveryPathError{CaptureZone: captureZone.Name, DeliveryZone: d.Zone.Name, Reason: fmt.Sprintf("EventConfig %q of zone %q does not mesh with delivery zone %q",
			ec.Name, captureZone.Name, d.Zone.Name)}
	}

	origin, err := CallbackOriginZone(captureZone, ec)
	if err != nil {
		return nil, "", err
	}
	var base string
	if SameZone(origin, d.Zone) {
		base = d.EventConfig.Status.CallbackURL
		if base == "" {
			return nil, "", &DeliveryPathError{CaptureZone: captureZone.Name, DeliveryZone: d.Zone.Name, Reason: fmt.Sprintf("delivery EventConfig %q has no CallbackURL in status", d.EventConfig.Name)}
		}
	} else {
		base = ec.Status.ProxyCallbackURLs[d.Zone.Name]
		if base == "" {
			return nil, "", &DeliveryPathError{CaptureZone: captureZone.Name, DeliveryZone: d.Zone.Name, Reason: fmt.Sprintf("capture EventConfig %q has no ProxyCallbackURLs entry for delivery zone %q",
				ec.Name, d.Zone.Name)}
		}
	}

	return &ListenerPlacement{
		CaptureZone:         captureZone,
		CaptureRoute:        route,
		CaptureEventConfig:  ec,
		CaptureEventStore:   es,
		CallbackOriginZone:  origin,
		DeliveryZone:        d.Zone,
		DeliveryEventConfig: d.EventConfig,
		DeliveryEventStore:  d.EventStore,
		BridgeNamespace:     es.Namespace,
		CallbackBaseURL:     base,
	}, "", nil
}
