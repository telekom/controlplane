// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package noop

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

// noopStore is a no-operation implementation of ObjectStore that returns
// empty results for read operations and errors for write operations.
// It is intended to be used when a feature is disabled, allowing callers
// to interact with the store without special-casing feature-flag checks.
type noopStore[T store.Object] struct {
	gvr schema.GroupVersionResource
	gvk schema.GroupVersionKind
}

// NewStore creates a new ObjectStore that performs no operations.
// Read operations return not-found or empty results, and write operations
// return an error indicating the feature is disabled.
func NewStore[T store.Object](gvr schema.GroupVersionResource, gvk schema.GroupVersionKind) store.ObjectStore[T] {
	return &noopStore[T]{
		gvr: gvr,
		gvk: gvk,
	}
}

func (s *noopStore[T]) Info() (schema.GroupVersionResource, schema.GroupVersionKind) {
	return s.gvr, s.gvk
}

func (s *noopStore[T]) Ready() bool {
	return true
}

func (s *noopStore[T]) Get(_ context.Context, _, name string) (T, error) {
	var zero T
	return zero, problems.NotFound(name)
}

func (s *noopStore[T]) List(_ context.Context, _ store.ListOpts) (*store.ListResponse[T], error) {
	return &store.ListResponse[T]{Items: []T{}}, nil
}

func (s *noopStore[T]) Delete(_ context.Context, _, name string) error {
	return problems.NotFound(name)
}

func (s *noopStore[T]) CreateOrReplace(_ context.Context, _ T) error {
	return problems.BadRequest(fmt.Sprintf("feature for %s is disabled", s.gvk.Kind))
}

func (s *noopStore[T]) Patch(_ context.Context, _, _ string, _ ...store.Patch) (T, error) {
	var zero T
	return zero, problems.BadRequest(fmt.Sprintf("feature for %s is disabled", s.gvk.Kind))
}
