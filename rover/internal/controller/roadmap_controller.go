// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // Single-resource controller scaffolds are intentionally kept parallel for clarity.
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

	apiapi "github.com/telekom/controlplane/api/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	rover "github.com/telekom/controlplane/rover/api/v1"
	roadmap_handler "github.com/telekom/controlplane/rover/internal/handler/roadmap"
)

// RoadmapReconciler reconciles a Roadmap object
type RoadmapReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.Roadmap]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=roadmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=roadmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=roadmaps/finalizers,verbs=update
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=apispecifications,verbs=get;list;watch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiroadmaps,verbs=get;list;watch;create;update;patch;delete

func (r *RoadmapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.Roadmap{})
}

// MapApiSpecificationToRoadmaps enqueues the roadmaps that describe the given ApiSpecification,
// so a roadmap applied before its specification is unblocked once the specification exists.
func (r *RoadmapReconciler) MapApiSpecificationToRoadmaps(ctx context.Context, obj client.Object) []reconcile.Request {
	list := &rover.RoadmapList{}
	// Namespace isolation is intended: every environment owns its own namespace.
	if err := r.List(ctx, list, client.InNamespace(obj.GetNamespace())); err != nil {
		log.FromContext(ctx).Error(err, "failed to list Roadmaps for ApiSpecification", "apiSpecification", obj.GetName())
		return nil
	}

	var requests []reconcile.Request
	for _, item := range list.Items {
		if item.Spec.SpecificationRef.Kind == "ApiSpecification" && item.Spec.SpecificationRef.Name == obj.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoadmapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("roadmap-controller")
	r.Controller = cc.NewController(&roadmap_handler.RoadmapHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&rover.Roadmap{}, builder.WithPredicates(cc.Count("roadmap", cc.RoleFor))).
		Owns(&apiapi.ApiRoadmap{}, builder.WithPredicates(cc.Count("roadmap", cc.RoleOwns))).
		Watches(&rover.ApiSpecification{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiSpecificationToRoadmaps),
			builder.WithPredicates(cc.Count("roadmap", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}, cc.SkipInitialListPredicate{})),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
