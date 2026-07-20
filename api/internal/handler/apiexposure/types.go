// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"slices"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/types"
)

// ──────────────────────────────────────────────────────────────────────────────
// Route Provisioning Pipeline
// ──────────────────────────────────────────────────────────────────────────────
//
// The route provisioning pipeline runs in three steps:
//
//  1. determineRoutingState — fetches subscribers and zone data, derives flags.
//  2. manageProxyRoutes    — creates proxy routes, collects consumer failover enrichment.
//  3. createRealRoute      — creates the real route using the enriched state.
//
// All steps share a single routingState instance to avoid redundant API calls
// and make the data flow between steps explicit.

// routingState holds pre-computed data used across the route provisioning pipeline.
//
// The distinction between "consumer failover" and "provider failover" is important:
//   - Consumer failover (a.k.a. DTC/DNS failover): all proxy routes AND the real route are
//     enriched with additional hostnames and IDP issuers from ALL zones that have the
//     ConsumerFailover feature enabled. This allows external DNS to switch consumers
//     between zone gateways transparently.
//   - Provider failover: creates a secondary route in a backup zone with the provider's
//     upstreams, allowing traffic to be served from a different zone if the provider fails.
//     Managed separately via WithFailoverUpstreams/WithFailoverSecurity.
type routingState struct {
	// ──────────────────────────────────────────────────────────────────────────
	// Determined up front by determineRoutingState
	// ──────────────────────────────────────────────────────────────────────────

	// realmName is the environment/realm name used for token validation on all routes.
	realmName string

	// subscribers is the full list of approved, non-deleted ApiSubscriptions for this exposure.
	// Used to determine which zones need proxy routes and whether consumer failover is active.
	subscribers []*apiapi.ApiSubscription

	// hasCrossZoneSubs is true if at least one subscriber is in a different zone than the exposure.
	// Drives: real route gets GatewayConsumerName in DefaultConsumers (mesh-client access).
	hasCrossZoneSubs bool

	// hasLocalSubs is true if at least one subscriber is in the same zone as the exposure.
	// Drives: real route trusts the exposure zone's own IDP issuer (direct consumer access).
	hasLocalSubs bool

	// hasConsumerFailover is true if at least one subscriber has consumer failover configured.
	// When true, ALL proxy routes and the real route are enriched with failover hostnames/issuers.
	hasConsumerFailover bool

	// exposureZone is the Zone object where the API is exposed (provider zone).
	// Used to read the zone's IDP issuer and to identify which zone is "self" in the failover loop.
	exposureZone *adminv1.Zone

	// resolvedClaims holds the exposure's M2M claims after static ValueFrom sources
	// (ProviderClientId, BasePath) have been resolved to literals using the application's
	// client id. Applied to the real route and provider failover routes.
	resolvedClaims *apiapi.Claims

	// ──────────────────────────────────────────────────────────────────────────
	// Consumer failover enrichment — produced by manageProxyRoutes
	// Applied to ALL proxy routes AND the real route.
	// Collected from every zone that has the ConsumerFailover feature enabled
	// (including the exposure zone itself).
	// ──────────────────────────────────────────────────────────────────────────

	// consumerFailoverHosts are the hostnames from the ConsumerFailover gateway presets of all
	// eligible zones. Added as additional hostnames so that any zone's gateway can accept
	// traffic for any other zone's failover hostname after a DNS switch.
	consumerFailoverHosts []string

	// consumerFailoverPaths are the paths from the ConsumerFailover gateway presets of all
	// eligible zones. Added alongside consumerFailoverHosts.
	consumerFailoverPaths []string

	// consumerFailoverIssuers are the IDP issuers from all eligible zones.
	// Added as trusted issuers so that when a consumer fails over to a different zone's
	// gateway, the route can validate the consumer's home-zone IDP token directly.
	consumerFailoverIssuers []string

	// ──────────────────────────────────────────────────────────────────────────
	// Mesh trust — produced by manageProxyRoutes
	// Only for the real route. NOT related to consumer failover.
	// ──────────────────────────────────────────────────────────────────────────

	// crossZoneLmsIssuers are the LMS (Last-Mile-Security) issuers from all non-exposure
	// zones that have proxy routes. The real route must trust these because proxy gateways
	// in other zones stamp an LMS token before forwarding traffic to the provider zone.
	// This is a mesh concern, not a consumer failover concern.
	crossZoneLmsIssuers map[types.ObjectRef]string
}

// CrossZoneLmsIssuers returns the cross-zone LMS issuers, optionally excluding
// the given zones (e.g. a failover zone whose issuer is already trusted through
// another path).
func (s *routingState) CrossZoneLmsIssuers(withoutZones ...types.ObjectRef) []string {
	res := make([]string, 0, len(s.crossZoneLmsIssuers))
	for zone, issuer := range s.crossZoneLmsIssuers {
		if slices.ContainsFunc(withoutZones, func(z types.ObjectRef) bool { return z.Equals(&zone) }) {
			continue
		}
		res = append(res, issuer)
	}
	slices.Sort(res)
	return slices.Clip(res)
}

// AddCrossZoneLmsIssuer records the LMS issuer for a proxy route's zone,
// lazily initializing the map on first use.
func (s *routingState) AddCrossZoneLmsIssuer(zone types.ObjectRef, issuer string) {
	if s.crossZoneLmsIssuers == nil {
		s.crossZoneLmsIssuers = make(map[types.ObjectRef]string)
	}
	s.crossZoneLmsIssuers[zone] = issuer
}
