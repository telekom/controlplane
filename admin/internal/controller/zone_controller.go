// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	zone_handler "github.com/telekom/controlplane/admin/internal/handler/zone"
	corev1 "k8s.io/api/core/v1"
	runtime_ctrl "sigs.k8s.io/controller-runtime/pkg/controller"
)

// ZoneReconciler reconciles a Zone object
type ZoneReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controller.Controller[*adminv1.Zone]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/finalizers,verbs=update
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=environments,verbs=get;list;watch

// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=identityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete

func (r *ZoneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &adminv1.Zone{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("zone-controller")
	r.Controller = controller.NewController(&zone_handler.ZoneHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&adminv1.Zone{}).
		Watches(&adminv1.Environment{},
			handler.EnqueueRequestsFromMapFunc(r.mapEnvironmentToZone),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Owns(&corev1.Namespace{}).
		WithOptions(runtime_ctrl.Options{
			MaxConcurrentReconciles: 1,
			// RateLimiter:             ...
		}).
		Complete(r)
}

func (r *ZoneReconciler) mapEnvironmentToZone(ctx context.Context, obj client.Object) []reconcile.Request {
	environment, ok := obj.(*adminv1.Environment)
	if !ok {
		return nil
	}

	list := &adminv1.ZoneList{}
	err := r.List(ctx, list, client.MatchingLabels{"environment": environment.GetName()})
	if err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, zone := range list.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&zone)})
	}

	return requests
}
