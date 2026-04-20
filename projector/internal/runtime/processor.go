// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Processor orchestrates the projection pipeline for a single resource type.
//
// Type parameters:
//   - T: Kubernetes object type
//   - D: typed domain payload (DTO)
//   - K: typed identity key
type Processor[T client.Object, D any, K any] struct {
	translator Translator[T, D, K]
	repository Repository[K, D]
}

// NewProcessor creates a Processor wired with the given translator and repository.
func NewProcessor[T client.Object, D any, K any](
	translator Translator[T, D, K],
	repository Repository[K, D],
) *Processor[T, D, K] {
	return &Processor[T, D, K]{
		translator: translator,
		repository: repository,
	}
}

// Upsert runs the sync pipeline for a live object: ShouldSkip → Translate → Repository.Upsert.
func (p *Processor[T, D, K]) Upsert(ctx context.Context, obj T) error {
	if skip, reason := p.translator.ShouldSkip(obj); skip {
		return fmt.Errorf("%s: %w", reason, ErrSkipSync)
	}
	data, err := p.translator.Translate(ctx, obj)
	if err != nil {
		return fmt.Errorf("translate: %w", err)
	}
	return p.repository.Upsert(ctx, data)
}

// Delete runs the delete pipeline: KeyFromDelete → Repository.Delete.
func (p *Processor[T, D, K]) Delete(ctx context.Context, req types.NamespacedName, lastKnown T) error {
	key, err := p.translator.KeyFromDelete(req, lastKnown)
	if err != nil {
		return fmt.Errorf("derive delete key: %w", err)
	}
	return p.repository.Delete(ctx, key)
}
