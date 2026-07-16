// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"

	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

const (
	// PublisherID is the fixed publisher identity used for all Spectre pubsub resources.
	PublisherID = "gateway"

	// GenericEventType is the base event type for all Spectre listeners.
	GenericEventType = "de.telekom.ei.listener"
)

// MakePublisherName returns a K8s-safe CR name for a Publisher given an event type.
func MakePublisherName(eventType string) string {
	return labelutil.NormalizeNameValue(eventType)
}

// MakeSubscriberName returns a K8s-safe CR name for a Subscriber given a subscriber ID.
func MakeSubscriberName(subscriberId string) string {
	return labelutil.NormalizeNameValue(subscriberId)
}

// MakeRouteListenerName builds a deterministic name for a RouteListener CR.
func MakeRouteListenerName(listenerApp, apiName, consumer, provider string) string {
	raw := fmt.Sprintf("%s--%s--%s--%s", listenerApp, apiName, consumer, provider)
	return labelutil.NormalizeNameValue(raw)
}

// MakeBridgeSubscriberId builds the subscriber ID for a bridge subscription.
func MakeBridgeSubscriberId(consumer, listenerApp, apiName, kind string) string {
	return fmt.Sprintf("%s--%s--%s--%s", consumer, listenerApp, apiName, kind)
}

// BuildListenerEventType constructs the per-application event type for a listener.
func BuildListenerEventType(applicationId string) string {
	return GenericEventType + "." + applicationId
}

// BuildBridgeCallbackURL constructs the full callback URL for a bridge subscription.
// It wraps the EventConfig's callback route URL with the autoevent endpoint.
func BuildBridgeCallbackURL(callbackRouteURL, appId string) string {
	return callbackRouteURL + "?callback=http://localhost:8080/autoevent?listener=" + appId
}
