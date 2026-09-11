// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package event

import (
	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

// MakeName generates a deterministic resource name for an event exposure or subscription.
// It combines the owner (application) name with the normalized event type name.
func MakeName(ownerName, eventType string) string {
	return ownerName + "--" + eventv1.MakeEventTypeName(eventType)
}
