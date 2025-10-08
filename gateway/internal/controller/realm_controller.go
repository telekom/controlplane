// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	realm_handler "github.com/telekom/controlplane/gateway/internal/handler/realm"
)

// RealmReconciler reconciles a Realm object
type RealmReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*gatewayv1.Realm]
}

// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=realms/finalizers,verbs=update

func (r *RealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &gatewayv1.Realm{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("realm-controller")
	r.Controller = cc.NewController(&realm_handler.RealmHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.Realm{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Owns(&gatewayv1.Route{}).
		Watches(&gatewayv1.Gateway{},
			handler.EnqueueRequestsFromMapFunc(NewMapGatewayToRealm(r.Client)),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func NewMapGatewayToRealm(c client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {

		gateway, ok := obj.(*gatewayv1.Gateway)
		if !ok {
			return nil
		}
		if gateway.Labels == nil {
			return nil
		}

		matchLabels := client.MatchingLabels{
			cconfig.EnvironmentLabelKey:   gateway.Labels[cconfig.EnvironmentLabelKey],
			cconfig.BuildLabelKey("zone"): gateway.Labels[cconfig.BuildLabelKey("zone")],
		}

		list := &gatewayv1.RealmList{}
		if err := c.List(ctx, list, matchLabels); err != nil {
			return nil
		}

		requests := make([]reconcile.Request, len(list.Items))
		for _, realm := range list.Items {
			if realm.Spec.Gateway != nil && realm.Spec.Gateway.Equals(gateway) {
				requests = append(requests, reconcile.Request{
					NamespacedName: client.ObjectKey{
						Name:      realm.Name,
						Namespace: realm.Namespace,
					},
				})
			}
		}

		return requests
	}
}
