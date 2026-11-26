// SPDX-FileCopyrightText: 2025 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// basePathKind describes a kind that references an Api by its base path.
// The same kind T is used for the controller's For object and for the objects
// that are enqueued when an Api changes.
type basePathKind[T client.Object, L client.ObjectList] struct {
	name     string
	newObj   func() T
	newList  func() L
	basePath func(T) string
}

// setupBasePathController registers a controller for kind K that also watches Apis,
// so objects that reference an Api by base path are re-reconciled when the Api changes
// (e.g. an object applied before its Api exists gets linked once the Api appears).
func setupBasePathController[T client.Object, L client.ObjectList](mgr ctrl.Manager, k basePathKind[T, L], r reconcile.Reconciler) error {
	c := mgr.GetClient()
	mapFn := func(ctx context.Context, obj client.Object) []reconcile.Request {
		return k.mapApi(ctx, c, obj)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(k.newObj(), builder.WithPredicates(cc.Count(k.name, cc.RoleFor))).
		Named(k.name).
		Watches(&apiv1.Api{},
			handler.EnqueueRequestsFromMapFunc(mapFn),
			builder.WithPredicates(cc.Count(k.name, cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}, cc.SkipInitialListPredicate{})),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// mapApi returns reconcile requests for all objects of kind T in the Api's namespace
// whose base path matches the Api's base path.
func (k basePathKind[T, L]) mapApi(ctx context.Context, c client.Reader, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	api, ok := obj.(*apiv1.Api)
	if !ok {
		logger.Info("object is not an Api")
		return nil
	}

	list := k.newList()
	// Namespace isolation is intended: every environment owns its own namespace.
	if err := c.List(ctx, list, client.InNamespace(api.Namespace)); err != nil {
		logger.Error(err, "failed to list objects for base path mapping")
		return nil
	}

	basePath := labelutil.NormalizeLabelValue(api.Spec.BasePath)
	objs, err := meta.ExtractList(list)
	if err != nil {
		logger.Error(err, "failed to extract list items")
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(objs))
	for _, o := range objs {
		item, ok := o.(T)
		if !ok || labelutil.NormalizeLabelValue(k.basePath(item)) != basePath {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(item)})
	}
	return reqs
}
