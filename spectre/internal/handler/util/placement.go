// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	"github.com/pkg/errors"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
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

	// CallbackOriginZone is the zone whose callback gateway URL is used for
	// delivery. During Phase 2 (A==C) this equals CaptureZone; Phase 3 may
	// resolve it independently for proxy scenarios.
	CallbackOriginZone *adminv1.Zone

	DeliveryZone        *adminv1.Zone
	DeliveryEventConfig *eventv1.EventConfig
	DeliveryEventStore  *pubsubv1.EventStore

	// BridgeNamespace is the Kubernetes namespace where bridge Subscribers and
	// the shared generic Publisher live. Derived from CaptureEventStore.Namespace.
	BridgeNamespace string

	// CallbackBaseURL is the external gateway URL used for event delivery
	// callbacks. Resolved from the capture EventConfig's Status.CallbackURL.
	CallbackBaseURL string
}

// ResolvePlacement resolves the EventConfig, EventStore, and callback URL for
// a Listener whose capture zone, observer zone, and gateway Route have already
// been determined.
//
// captureZone is the zone where traffic is intercepted (from GetListeningZone).
// observerZone is A's zone — the SpectreApplication owner — where delivery
// always happens.
//
// When captureZone == observerZone (same-zone / A==C on the same path), a
// single EventConfig and EventStore serve both roles and the local CallbackURL
// is used. When the zones differ (cross-zone), separate EventConfigs and
// EventStores are resolved and the delivery EventConfig's ProxyCallbackURLs
// map supplies the callback URL keyed by the capture zone name.
func ResolvePlacement(
	ctx context.Context,
	captureZone *adminv1.Zone,
	observerZone *adminv1.Zone,
	route *gatewayv1.Route,
) (*ListenerPlacement, error) {
	// Step 1: Resolve capture-side EventConfig and EventStore.
	captureEventConfig, err := GetEventConfig(ctx, captureZone)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to get capture EventConfig")
	}
	captureEventStore, err := ResolveEventStore(ctx, captureEventConfig)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to resolve capture EventStore")
	}

	// Same-zone fast path: capture and delivery share the same infrastructure.
	if captureZone.Name == observerZone.Name {
		if captureEventConfig.Status.CallbackURL == "" {
			return nil, ctrlerrors.BlockedErrorf(
				"placement: EventConfig %q has no CallbackURL in status", captureEventConfig.Name)
		}
		return &ListenerPlacement{
			CaptureZone:         captureZone,
			CaptureRoute:        route,
			CaptureEventConfig:  captureEventConfig,
			CaptureEventStore:   captureEventStore,
			CallbackOriginZone:  captureZone,
			DeliveryZone:        captureZone,
			DeliveryEventConfig: captureEventConfig,
			DeliveryEventStore:  captureEventStore,
			BridgeNamespace:     captureEventStore.Namespace,
			CallbackBaseURL:     captureEventConfig.Status.CallbackURL,
		}, nil
	}

	// Cross-zone: resolve delivery-side independently from observer zone.
	deliveryEventConfig, err := GetEventConfig(ctx, observerZone)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to get delivery EventConfig for observer zone")
	}
	deliveryEventStore, err := ResolveEventStore(ctx, deliveryEventConfig)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to resolve delivery EventStore for observer zone")
	}

	// Cross-zone callback: use ProxyCallbackURLs from the delivery EventConfig
	// keyed by the capture zone name.
	if deliveryEventConfig.Status.ProxyCallbackURLs == nil {
		return nil, ctrlerrors.BlockedErrorf(
			"placement: cross-zone delivery EventConfig %q has no ProxyCallbackURLs", deliveryEventConfig.Name)
	}
	callbackURL, ok := deliveryEventConfig.Status.ProxyCallbackURLs[captureZone.Name]
	if !ok {
		return nil, ctrlerrors.BlockedErrorf(
			"placement: cross-zone delivery EventConfig %q has no proxy callback for capture zone %q",
			deliveryEventConfig.Name, captureZone.Name)
	}

	return &ListenerPlacement{
		CaptureZone:         captureZone,
		CaptureRoute:        route,
		CaptureEventConfig:  captureEventConfig,
		CaptureEventStore:   captureEventStore,
		CallbackOriginZone:  captureZone,
		DeliveryZone:        observerZone,
		DeliveryEventConfig: deliveryEventConfig,
		DeliveryEventStore:  deliveryEventStore,
		BridgeNamespace:     captureEventStore.Namespace,
		CallbackBaseURL:     callbackURL,
	}, nil
}
