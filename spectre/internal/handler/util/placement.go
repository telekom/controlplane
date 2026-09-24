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

	// CallbackOriginZone is the zone whose Horizon makes the bridge callback:
	// the capture zone (see CallbackOriginZone).
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

// NoCaptureCandidateError reports that no candidate zone supports capture. It is
// a BlockedError: the Listener waits until the topology changes.
type NoCaptureCandidateError struct {
	ApiBasePath string
	Rejections  []CandidateRejection
}

func (e *NoCaptureCandidateError) Error() string {
	parts := make([]string, 0, len(e.Rejections))
	for _, r := range e.Rejections {
		parts = append(parts, fmt.Sprintf("zone %q: %s", r.Zone, r.Reason))
	}
	return fmt.Sprintf("no supported capture zone for path %q: %s", e.ApiBasePath, strings.Join(parts, "; "))
}

// IsBlocked implements ctrlerrors.BlockedError.
func (e *NoCaptureCandidateError) IsBlocked() bool { return true }

// CaptureCandidateZones returns the capture candidates in evaluation order:
// the observer (A) zone when it can be on the consumer (C) to provider (P)
// traffic path, then C's zone, then P's zone, deduplicated by zone identity.
//
// Gateway traffic from C to P only transits C's zone and P's zone, so A's zone
// is a candidate only when it is one of them. Any other A zone is rejected
// without being probed: a Route there serves other subscribers.
func CaptureCandidateZones(observerZone, consumerZone, providerZone *adminv1.Zone) ([]*adminv1.Zone, []CandidateRejection) {
	var rejections []CandidateRejection
	ordered := make([]*adminv1.Zone, 0, 3)
	if SameZone(observerZone, consumerZone) || SameZone(observerZone, providerZone) {
		ordered = append(ordered, observerZone)
	} else {
		rejections = append(rejections, CandidateRejection{
			Zone:   observerZone.Name,
			Reason: "observer zone is neither the consumer nor the provider zone and is not on the consumer→provider traffic path",
		})
	}
	ordered = append(ordered, consumerZone, providerZone)

	candidates := make([]*adminv1.Zone, 0, len(ordered))
	for _, z := range ordered {
		dup := false
		for _, c := range candidates {
			if SameZone(c, z) {
				dup = true
				break
			}
		}
		if !dup {
			candidates = append(candidates, z)
		}
	}
	return candidates, rejections
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

// CallbackOriginZone returns the zone whose Horizon makes the bridge callback
// for events captured in captureZone: the capture zone itself. The bridge
// Subscribers live in the capture EventStore; a proxy zone has its own
// EventStore CR, so this holds for local and proxy-backed capture alike and is
// never the proxy's target zone.
//
// The event domain builds EventConfig S's Status.ProxyCallbackURLs[T] as a
// Route on S's gateway whose upstream is T's gateway /horizon-T/callback/v1,
// T's primary callback Route, whose DynamicUpstream makes the final hop from
// T's gateway. A cross-zone bridge callback therefore uses the origin
// EventConfig's entry for the delivery zone: origin gateway -> A's gateway ->
// localhost:8080 at A's Jumper. The subscriber-keyed lookup the event domain
// uses for external callback URLs would end at the origin zone's Jumper.
func CallbackOriginZone(captureZone *adminv1.Zone, captureEC *eventv1.EventConfig) (*adminv1.Zone, error) {
	if captureEC.Spec.Zone.Name != captureZone.Name {
		return nil, errors.Errorf("placement: EventConfig %q belongs to zone %q, not capture zone %q",
			captureEC.Name, captureEC.Spec.Zone.Name, captureZone.Name)
	}
	return captureZone, nil
}

// ResolveCaptureZone resolves capture in captureZone for delivery d. It returns
// the placement on success, a non-empty reason when the zone is definitively
// unsuitable (no EventConfig, mesh excludes the delivery zone, no callback path
// to A), or an error for transient or ambiguous state (read errors, NotReady
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
		return nil, fmt.Sprintf("EventConfig %q of zone %q does not mesh with delivery zone %q",
			ec.Name, captureZone.Name, d.Zone.Name), nil
	}

	origin, err := CallbackOriginZone(captureZone, ec)
	if err != nil {
		return nil, "", err
	}
	var base string
	if SameZone(origin, d.Zone) {
		base = d.EventConfig.Status.CallbackURL
		if base == "" {
			return nil, fmt.Sprintf("delivery EventConfig %q has no CallbackURL in status", d.EventConfig.Name), nil
		}
	} else {
		base = ec.Status.ProxyCallbackURLs[d.Zone.Name]
		if base == "" {
			return nil, fmt.Sprintf("capture EventConfig %q has no ProxyCallbackURLs entry for delivery zone %q",
				ec.Name, d.Zone.Name), nil
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
