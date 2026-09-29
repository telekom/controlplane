// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"context"
	"fmt"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/approval"
	"github.com/telekom/controlplane/projector/internal/domain/approvalrequest"
	"github.com/telekom/controlplane/projector/internal/runtime"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listenerProcessor removes projections that can no longer be represented and
// restores child projections lost to the database's Listener delete cascades.
type listenerProcessor struct {
	processor  *runtime.Processor[*spectrev1.Listener, *Data, Key]
	translator *Translator
	repo       *Repository
	recovery   *approvalRecovery
}

func (p *listenerProcessor) Upsert(ctx context.Context, obj *spectrev1.Listener) error {
	if skip, _ := p.translator.ShouldSkip(obj); skip {
		if err := p.repo.Delete(ctx, p.translator.KeyFromObject(obj)); err != nil {
			return err
		}
		return p.processor.Upsert(ctx, obj)
	}
	if err := p.processor.Upsert(ctx, obj); err != nil {
		return err
	}
	return p.recovery.restore(ctx, p.translator.KeyFromObject(obj))
}

func (p *listenerProcessor) Delete(ctx context.Context, req types.NamespacedName, lastKnown *spectrev1.Listener) error {
	return p.processor.Delete(ctx, req, lastKnown)
}

type approvalRecovery struct {
	reader    client.Reader
	approvals *approval.Repository
	requests  *approvalrequest.Repository
}

func (r *approvalRecovery) restore(ctx context.Context, key Key) error {
	selector := client.MatchingFields{approvalTargetIndex: key.Namespace + "/" + key.Name}
	approvals := &approvalv1.ApprovalList{}
	if err := r.reader.List(ctx, approvals, selector); err != nil {
		return fmt.Errorf("list approvals for listener %s/%s: %w", key.Namespace, key.Name, err)
	}
	translator := &approval.Translator{}
	for i := range approvals.Items {
		obj := &approvals.Items[i]
		if skip, _ := translator.ShouldSkip(obj); skip {
			continue
		}
		data, err := translator.Translate(ctx, obj)
		if err != nil {
			return fmt.Errorf("translate approval %s/%s: %w", obj.Namespace, obj.Name, err)
		}
		if err := r.approvals.Upsert(ctx, data); err != nil {
			return fmt.Errorf("restore approval %s/%s: %w", obj.Namespace, obj.Name, err)
		}
	}
	requests := &approvalv1.ApprovalRequestList{}
	if err := r.reader.List(ctx, requests, selector); err != nil {
		return fmt.Errorf("list approval requests for listener %s/%s: %w", key.Namespace, key.Name, err)
	}
	requestTranslator := &approvalrequest.Translator{}
	for i := range requests.Items {
		obj := &requests.Items[i]
		if skip, _ := requestTranslator.ShouldSkip(obj); skip {
			continue
		}
		data, err := requestTranslator.Translate(ctx, obj)
		if err != nil {
			return fmt.Errorf("translate approval request %s/%s: %w", obj.Namespace, obj.Name, err)
		}
		if err := r.requests.Upsert(ctx, data); err != nil {
			return fmt.Errorf("restore approval request %s/%s: %w", obj.Namespace, obj.Name, err)
		}
	}
	return nil
}
