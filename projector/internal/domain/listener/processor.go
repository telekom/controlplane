// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"

	"github.com/telekom/controlplane/projector/internal/runtime"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"k8s.io/apimachinery/pkg/types"
)

// listenerProcessor removes projections that can no longer be represented.
type listenerProcessor struct {
	processor  *runtime.Processor[*spectrev1.Listener, *Data, Key]
	translator *Translator
	repo       *Repository
}

func (p *listenerProcessor) Upsert(ctx context.Context, obj *spectrev1.Listener) error {
	if skip, _ := p.translator.ShouldSkip(obj); skip {
		// Removes only this object's projected PostgreSQL row; no Kubernetes object is deleted.
		if err := p.repo.Delete(ctx, p.translator.KeyFromObject(obj)); err != nil {
			return err
		}
	}
	return p.processor.Upsert(ctx, obj)
}

func (p *listenerProcessor) Delete(ctx context.Context, req types.NamespacedName, lastKnown *spectrev1.Listener) error {
	return p.processor.Delete(ctx, req, lastKnown)
}
