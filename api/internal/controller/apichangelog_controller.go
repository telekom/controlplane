// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/apichangelog"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// ApiChangelogReconciler reconciles a ApiChangelog object
type ApiChangelogReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*apiv1.ApiChangelog]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apichangelogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apichangelogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apichangelogs/finalizers,verbs=update

func (r *ApiChangelogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &apiv1.ApiChangelog{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApiChangelogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("apichangelog-controller")
	r.Controller = cc.NewController(&apichangelog.ApiChangelogHandler{Client: r.Client}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.ApiChangelog{}, builder.WithPredicates(cc.Count("apichangelog", cc.RoleFor))).
		Named("apichangelog").
		Watches(&apiv1.Api{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiToApiChangelog),
			builder.WithPredicates(cc.Count("apichangelog", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}, cc.SkipInitialListPredicate{})),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapApiToApiChangelog enqueues the changelogs that describe the given Api,
// so that a changelog applied before its Api exists gets linked once the Api appears.
func (r *ApiChangelogReconciler) MapApiToApiChangelog(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	api, ok := obj.(*apiv1.Api)
	if !ok {
		logger.Info("object is not an Api")
		return nil
	}

	list := &apiv1.ApiChangelogList{}
	// Namespace isolation is intended: every environment owns its own namespace.
	if err := r.List(ctx, list, client.InNamespace(api.Namespace)); err != nil {
		logger.Error(err, "failed to list ApiChangelogs")
		return nil
	}

	basePath := labelutil.NormalizeLabelValue(api.Spec.BasePath)
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		if labelutil.NormalizeLabelValue(item.Spec.BasePath) != basePath {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(item)})
	}
	return reqs
}
