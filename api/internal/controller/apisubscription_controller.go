// Copyright 2025 Deutsche Telekom IT GmbH
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
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	"github.com/telekom/controlplane/api/internal/handler/apisubscription"
)

// ApiSubscriptionReconciler reconciles a ApiSubscription object
type ApiSubscriptionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*apiapi.ApiSubscription]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apisubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apisubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apisubscriptions/finalizers,verbs=update
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis,verbs=get;list;watch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis/status,verbs=get
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures,verbs=get;list;watch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures/status,verbs=get
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvals,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumeroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch

func (r *ApiSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, new(apiapi.ApiSubscription))
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApiSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("apisubscription-controller")
	r.Controller = cc.NewController(&apisubscription.ApiSubscriptionHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiapi.ApiSubscription{}).
		Owns(&approvalapi.ApprovalRequest{}).
		Owns(&approvalapi.Approval{}).
		Owns(&gatewayapi.ConsumeRoute{}).
		Owns(&apiapi.RemoteApiSubscription{}).
		Watches(&apiapi.Api{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiToApiSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&apiapi.ApiExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapApiExposureToApiSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&applicationapi.Application{},
			handler.EnqueueRequestsFromMapFunc(r.MapApplicationToApiSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&gatewayv1.Route{},
			handler.EnqueueRequestsFromMapFunc(r.MapRouteToApiSubscription),
			builder.WithPredicates(DeleteOnlyPredicate{}),
		).
		Watches(&gatewayv1.ConsumeRoute{},
			handler.EnqueueRequestsFromMapFunc(r.MapConsumeRouteToApiSubscription),
			builder.WithPredicates(DeleteOnlyPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter()}).
		Complete(r)
}

func (r *ApiSubscriptionReconciler) MapApiToApiSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	api, ok := obj.(*apiapi.Api)
	if !ok {
		log.Info("object is not an API")
		return nil
	}

	list := &apiapi.ApiSubscriptionList{}
	err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: api.Labels[cconfig.EnvironmentLabelKey],
		apiapi.BasePathLabelKey:     api.Labels[apiapi.BasePathLabelKey],
	})
	if err != nil {
		log.Error(err, "failed to list API-Subscriptions")
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

func (r *ApiSubscriptionReconciler) MapApiExposureToApiSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	apiExposure, ok := obj.(*apiapi.ApiExposure)
	if !ok {
		log.Info("object is not an API-Exposure")
		return nil
	}

	list := &apiapi.ApiSubscriptionList{}
	err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey: apiExposure.Labels[cconfig.EnvironmentLabelKey],
		apiapi.BasePathLabelKey:     apiExposure.Labels[apiapi.BasePathLabelKey],
	})
	if err != nil {
		log.Error(err, "failed to list API-Subscriptions")
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

func (r *ApiSubscriptionReconciler) MapApplicationToApiSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	application, ok := obj.(*applicationapi.Application)
	if !ok {
		log.Info("object is not an Application")
		return nil
	}

	list := &apiapi.ApiSubscriptionList{}
	err := r.Client.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:          application.Labels[cconfig.EnvironmentLabelKey],
		cconfig.BuildLabelKey("application"): application.Labels[cconfig.BuildLabelKey("application")],
	}, client.InNamespace(application.Namespace))
	if err != nil {
		log.Error(err, "failed to list API-Subscriptions")
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
	}

	return reqs
}

func (r *ApiSubscriptionReconciler) MapRouteToApiSubscription(ctx context.Context, obj client.Object) []reconcile.Request {

	return nil
}

func (r *ApiSubscriptionReconciler) MapConsumeRouteToApiSubscription(ctx context.Context, obj client.Object) []reconcile.Request {

	return nil
}

var _ predicate.Predicate = DeleteOnlyPredicate{}

// DeleteOnlyPredicate implements a predicate that only processes DELETE events
type DeleteOnlyPredicate struct {
	predicate.Funcs
}

func (DeleteOnlyPredicate) Create(e event.CreateEvent) bool {
	return false
}

func (DeleteOnlyPredicate) Delete(e event.DeleteEvent) bool {
	return true
}

func (DeleteOnlyPredicate) Update(e event.UpdateEvent) bool {
	return false
}

func (DeleteOnlyPredicate) Generic(e event.GenericEvent) bool {
	return false
}
