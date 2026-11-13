// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

const (
	// CallbackClientName is the client Horizon uses to send callback requests.
	// Callback routes (primary and proxy) must trust this client. It is NOT a
	// mesh/proxy client: cross-zone meshing is done via gatewayapi.GatewayConsumerName.
	CallbackClientName = "eventstore"
	// AdminClientName is the name of the client used for administrative operations.
	// This client must be configured in the configuration backend.
	AdminClientName = "horizon-quasar"

	// CallbackURLQueryParam is the name of the query parameter used to pass the original callback URL in proxy scenarios.
	CallbackURLQueryParam = "callback"

	// horizon is the leading segment of all event route paths and Route names.
	// Route names must equal the legacy-compatible Kong base names, so do not
	// change it without migrating the existing Kong entities.
	horizon = "horizon"
)

const (
	LabelValueDomain = "event"
)

// horizonPrefix builds the leading path segment for an event route.
// An empty zone yields the local alias "/horizon"; a zone yields the mesh
// form "/horizon-{zone}". This is the single place that encodes the
// local-vs-mesh alias rule.
func horizonPrefix(zoneName string) string {
	if zoneName == "" {
		return "/" + horizon
	}
	return "/" + horizon + "-" + zoneName
}

func makePublishRouteName() string {
	return horizon + "-events"
}

// makePublishEventsRoutePath returns the main downstream path of the publish route.
func makePublishEventsRoutePath() string {
	return horizonPrefix("") + "/events/v1"
}

// makePublishRoutePath returns the secondary downstream path of the publish route.
func makePublishRoutePath() string {
	return horizonPrefix("") + "/publish/v1"
}

// makeSSERouteName returns the deterministic Route name for an SSE event type.
func makeSSERouteName(eventType string) string {
	return labelutil.NormalizeNameValue(horizon + "-sse--" + eventv1.MakeEventTypeName(eventType))
}

// makeSSERoutePath returns the (zone-independent) SSE path for an event type,
// e.g. "/horizon/sse/v1/de.telekom.eni.quickstart.v1".
func makeSSERoutePath(eventType string) string {
	return horizonPrefix("") + "/sse/v1/" + strings.ToLower(eventType)
}

func makeCallbackRouteName(zoneName string) string {
	return horizon + "-callback-" + zoneName
}

func makeCallbackRoutePath(zoneName string) string {
	return horizonPrefix(zoneName) + "/callback/v1"
}

func makeVoyagerRouteName(zoneName string) string {
	return horizon + "-voyager-" + zoneName
}

func makeVoyagerRoutePath(zoneName string) string {
	return horizonPrefix(zoneName) + "/voyager/v1"
}
