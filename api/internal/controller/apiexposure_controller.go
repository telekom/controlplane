// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/apiexposure"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
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
)

// ApiExposureReconciler reconciles a ApiExposure object
type ApiExposureReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*apiv1.ApiExposure]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures/finalizers,verbs=update
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis,verbs=get;list;watch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis/status,verbs=get
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete

func (r *ApiExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &apiv1.ApiExposure{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApiExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("apiexposure-controller")
	r.Controller = cc.NewController(&apiexposure.ApiExposureHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1.ApiExposure{}).
		Watches(&apiv1.Api{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiToApiExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&apiv1.ApiExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiExposureToApiExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&gatewayv1.Route{},
			handler.EnqueueRequestsFromMapFunc(r.MapRouteToApiExposure),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToApiExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter()}).
		Complete(r)
}

func (r *ApiExposureReconciler) MapApiToApiExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)
	api, ok := obj.(*apiv1.Api)
	if !ok {
		log.Info("object is not an API")
		return nil
	}

	list := &apiv1.ApiExposureList{}
	err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: api.Labels[cconfig.EnvironmentLabelKey],
		apiv1.BasePathLabelKey:      api.Labels[apiv1.BasePathLabelKey],
	})
	if err != nil {
		log.Error(err, "failed to list API-Exposures")
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		if api.UID == item.UID {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
	}

	return reqs
}

func (r *ApiExposureReconciler) MapApiExposureToApiExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)
	apiExposure, ok := obj.(*apiv1.ApiExposure)
	if !ok {
		log.Info("object is not an ApiExposure")
		return nil
	}

	list := &apiv1.ApiExposureList{}
	err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: apiExposure.Labels[cconfig.EnvironmentLabelKey],
		apiv1.BasePathLabelKey:      apiExposure.Labels[apiv1.BasePathLabelKey],
	})
	if err != nil {
		log.Error(err, "failed to list API-Exposures")
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		if apiExposure.UID == item.UID {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
	}

	return reqs
}

func (r *ApiExposureReconciler) MapRouteToApiExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	return nil
}

func (r *ApiExposureReconciler) MapZoneToApiExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	return nil
}
