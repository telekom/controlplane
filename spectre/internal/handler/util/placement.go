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
// a Listener whose listening zone and gateway Route have already been determined.
//
// The caller is responsible for calling GetListeningZone and findRouteByPath
// beforehand so that route-mode rejection (pass-through, failover) can happen
// before this more expensive resolution. ResolvePlacement then resolves the
// remaining infrastructure: EventConfig, EventStore, callback URL.
//
// During Phase 2 (A==C enforced), capture and delivery are co-located: the
// listening zone serves both roles and the callback origin is the capture zone.
func ResolvePlacement(
	ctx context.Context,
	listeningZone *adminv1.Zone,
	route *gatewayv1.Route,
) (*ListenerPlacement, error) {
	// Step 1: Resolve EventConfig for the listening zone.
	eventConfig, err := GetEventConfig(ctx, listeningZone)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to get EventConfig")
	}

	if eventConfig.Status.CallbackURL == "" {
		return nil, ctrlerrors.BlockedErrorf(
			"placement: EventConfig %q has no CallbackURL in status", eventConfig.Name)
	}

	// Step 3: Resolve EventStore via EventConfig reference.
	eventStore, err := ResolveEventStore(ctx, eventConfig)
	if err != nil {
		return nil, errors.Wrap(err, "placement: failed to resolve EventStore")
	}

	// Phase 2: capture == delivery, callback origin == capture zone.
	return &ListenerPlacement{
		CaptureZone:         listeningZone,
		CaptureRoute:        route,
		CaptureEventConfig:  eventConfig,
		CaptureEventStore:   eventStore,
		CallbackOriginZone:  listeningZone,
		DeliveryZone:        listeningZone,
		DeliveryEventConfig: eventConfig,
		DeliveryEventStore:  eventStore,
		BridgeNamespace:     eventStore.Namespace,
		CallbackBaseURL:     eventConfig.Status.CallbackURL,
	}, nil
}
