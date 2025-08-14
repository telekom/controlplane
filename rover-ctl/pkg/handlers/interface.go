// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

type Sortable interface {
	// Priority returns the priority of the sortable item
	Priority() int
}

// ResourceHandler defines the interface for handling different resource types
type ResourceHandler interface {
	Sortable
	Waiter

	// Apply creates or updates a resource on the server
	Apply(ctx context.Context, obj types.Object) error

	// Status retrieves the status of a resource from the server
	Status(ctx context.Context, name string) (types.ObjectStatus, error)

	// Delete removes a resource from the server
	Delete(ctx context.Context, obj types.Object) error

	// Get retrieves a resource by name from the server
	Get(ctx context.Context, name string) (any, error)

	// List retrieves all resources of this type from the server
	List(ctx context.Context) ([]any, error)

	Info(ctx context.Context, name string) (any, error)
}

type Waiter interface {
	// WaitForReady waits for a resource to be ready
	WaitForReady(ctx context.Context, name string) (types.ObjectStatus, error)

	// WaitForDeleted waits for a resource to be deleted
	WaitForDeleted(ctx context.Context, name string) (types.ObjectStatus, error)
}
