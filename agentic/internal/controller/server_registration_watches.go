// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

func registrationWatchSelector(obj client.Object, basePath string) (client.ListOption, error) {
	paths := []string{labelutil.NormalizeLabelValue(basePath)}
	if label, ok := obj.GetLabels()[agenticv1.BasePathLabelKey]; ok && label != paths[0] {
		paths = append(paths, label)
	}
	pathRequirement, err := labels.NewRequirement(agenticv1.BasePathLabelKey, selection.In, paths)
	if err != nil {
		return nil, fmt.Errorf("building registration basePath selector: %w", err)
	}
	selector := labels.NewSelector().Add(*pathRequirement)
	return client.MatchingLabelsSelector{Selector: selector}, nil
}

func enqueueMappedObjects(mapFunc func(context.Context, client.Object) []reconcile.Request) handler.EventHandler {
	enqueue := func(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request], objs ...client.Object) {
		for _, obj := range objs {
			for _, req := range mapFunc(ctx, obj) {
				q.Add(req)
			}
		}
	}

	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.ObjectOld, e.ObjectNew)
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(ctx, q, e.Object)
		},
	}
}
