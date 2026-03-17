// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"strings"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

const (
	// MeshClientName is the name of the client used for mesh communication.
	// It is used for both SSE and Callback proxy-routes to access the real-route.
	MeshClientName = "eventstore"
	// AdminClientName is the name of the client used for administrative operations.
	// This client must be configured in the configuration backend.
	AdminClientName = "admin--controlplane-client"

	// CallbackURLQueryParam is the name of the query parameter used to pass the original callback URL in proxy scenarios.
	CallbackURLQueryParam = "callback"
)

func makePublishRouteName(eventConfig *eventv1.EventConfig) string {
	return "publish"
}

func makePublishRoutePath(zoneName string) string {
	return fmt.Sprintf("/%s/publish/v1", zoneName)
}

// makeSSERouteName returns the deterministic Route name for an SSE event type.
func makeSSERouteName(eventType string) string {
	return "sse--" + eventv1.MakeEventTypeName(eventType)
}

func makeSSERoutePath(eventType string) string {
	return fmt.Sprintf("/sse/v1/%s", strings.ToLower(eventType)) // e.g. "/sse/v1/de.telekom.eni.quickstart.v1"
}

func makeCallbackRouteName(zoneName string) string {
	return "callback--" + zoneName
}

func makeCallbackRoutePath(zoneName string) string {
	return fmt.Sprintf("/%s/callback/v1", zoneName)
}

func makeVoyagerRouteName(zoneName string) string {
	return "voyager--" + zoneName
}

func makeVoyagerRoutePath(zoneName string) string {
	if zoneName == "" {
		return "/voyager/v1"
	}
	return fmt.Sprintf("/%s/voyager/v1", zoneName)
}
