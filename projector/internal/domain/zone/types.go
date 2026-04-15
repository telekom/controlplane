// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package zone implements the Zone resource module for the projector.
// Zone is a root entity (Level 0) with no FK dependencies.
package zone

import "github.com/telekom/controlplane/projector/internal/domain/shared"

// ZoneKey is the identity key for Zone entities, derived from metadata.name.
// Zone uses the Strong delete strategy — the key is always derivable from
// the reconcile request without needing lastKnown state.
type ZoneKey string

// ZoneData carries the transformed data for a Zone entity.
type ZoneData struct {
	Meta       shared.Metadata
	Name       string
	GatewayURL *string // optional/nillable — nil when Spec.Gateway.Url is empty
	Visibility string  // "WORLD" or "ENTERPRISE" (upper-cased from CR enum)
}
