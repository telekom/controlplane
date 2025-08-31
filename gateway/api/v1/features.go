// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

type FeatureType string

// Independent Features
const (
	FeatureTypePassThrough          FeatureType = "PassThrough"
	FeatureTypeAccessControl        FeatureType = "AccessControl"
	FeatureTypeRateLimit            FeatureType = "RateLimit"
	FeatureTypeHeaderTransformation FeatureType = "HeaderTransformation"
	FeatureTypeBasicAuth            FeatureType = "BasicAuth"
	FeatureTypeIpRestriction        FeatureType = "IpRestriction"
	FeatureTypeCircuitBreaker       FeatureType = "CircuitBreaker"
)

// Dependent Features
const (
	FeatureTypeLastMileSecurity FeatureType = "LastMileSecurity" // depends on AccessControl
	FeatureTypeExternalIDP      FeatureType = "ExternalIDP"      // depends on LastMileSecurity
	FeatureTypeCustomScopes     FeatureType = "CustomScopes"     // depends on LastMileSecurity
	FeatureTypeLoadBalancing    FeatureType = "LoadBalancing"    // depends on LastMileSecurity
	FeatureTypeFailover         FeatureType = "Failover"         // depends on LastMileSecurity
)
