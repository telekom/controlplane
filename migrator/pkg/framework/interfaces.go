// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceMigrator defines the interface for migrating a specific resource type
type ResourceMigrator interface {
	// GetName returns the unique name of this migrator (e.g., "approvalrequest")
	GetName() string

	// GetNewResourceType returns an empty instance of the new cluster resource
	GetNewResourceType() client.Object

	// GetLegacyAPIGroup returns the legacy API group (e.g., "acp.ei.telekom.de")
	GetLegacyAPIGroup() string

	// ComputeLegacyIdentifier computes the legacy namespace and name
	// Returns: namespace, name, skip (if true, migration is skipped), error
	ComputeLegacyIdentifier(ctx context.Context, obj client.Object) (namespace, name string, skip bool, err error)

	// FetchFromLegacy fetches the legacy resource from remote cluster
	FetchFromLegacy(ctx context.Context, remoteClient client.Client, namespace, name string) (client.Object, error)

	// HasChanged checks if the legacy resource state differs from current
	HasChanged(ctx context.Context, current, legacy client.Object) bool

	// ApplyMigration applies the legacy state to the current resource
	ApplyMigration(ctx context.Context, current, legacy client.Object) error

	// GetRequeueAfter returns the duration to wait before next reconciliation
	GetRequeueAfter() time.Duration
}

// MigrationOptions holds configuration for a migrator
type MigrationOptions struct {
	RequeueAfter time.Duration
	MaxRetries   int
}
