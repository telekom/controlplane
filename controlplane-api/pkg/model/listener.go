// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package model

// ListenerFilter describes conditions and payload paths applied by a Listener.
type ListenerFilter struct {
	Trigger map[string]string `json:"trigger,omitempty"`
	Payload []string          `json:"payload,omitempty"`
}
