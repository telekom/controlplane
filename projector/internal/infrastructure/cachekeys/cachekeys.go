// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cachekeys

// Zone returns the cache key components for a Zone entity.
// Zone names are globally unique, so the name alone is the lookup key.
func Zone(name string) (entityType, lookupKey string) {
	return "zone", name
}

// Group returns the cache key components for a Group entity.
// Group names are globally unique, so the name alone is the lookup key.
func Group(name string) (entityType, lookupKey string) {
	return "group", name
}

// Team returns the cache key components for a Team entity.
// Team names are globally unique, so the name alone is the lookup key.
func Team(name string) (entityType, lookupKey string) {
	return "team", name
}

// Application returns the cache key components for an Application entity.
// Application names are unique per team (composite unique index on
// name + owner_team), so both are required.
func Application(name, teamName string) (entityType, lookupKey string) {
	return "application", name + ":" + teamName
}

// APIExposure returns the cache key components for an ApiExposure entity
// identified by base path, application name, and team name.
// Base paths are unique per application, and applications per team,
// so all three are required.
func APIExposure(basePath, appName, teamName string) (entityType, lookupKey string) {
	return "apiexposure", basePath + ":" + appName + ":" + teamName
}

// APIExposureByBasePath returns the cache key components for an ApiExposure
// entity looked up by base path alone. Uses a "bp:" prefix to avoid collisions
// with the full composite key used by [APIExposure].
func APIExposureByBasePath(basePath string) (entityType, lookupKey string) {
	return "apiexposure", "bp:" + basePath
}

// APISubscriptionMeta returns the cache key components for an ApiSubscription
// entity looked up by its Kubernetes metadata (namespace + name). This enables
// Approval/ApprovalRequest to resolve the parent subscription FK from a
// spec.target reference.
func APISubscriptionMeta(namespace, name string) (entityType, lookupKey string) {
	return "apisubscription", "meta:" + namespace + ":" + name
}

// Approval returns the cache key components for an Approval entity,
// keyed by the Approval CR's Kubernetes namespace and name.
func Approval(namespace, name string) (entityType, lookupKey string) {
	return "approval", namespace + ":" + name
}

// ApprovalRequest returns the cache key components for an ApprovalRequest
// entity, keyed by the ApprovalRequest CR's Kubernetes namespace and name.
func ApprovalRequest(namespace, name string) (entityType, lookupKey string) {
	return "approvalrequest", namespace + ":" + name
}
