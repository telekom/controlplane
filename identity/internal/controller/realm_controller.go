// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/config"
	commonController "github.com/telekom/controlplane/common/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	realmHandler "github.com/telekom/controlplane/identity/internal/handler/realm"
)

// RealmReconciler reconciles a Realm object
type RealmReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	commonController.Controller[*identityv1.Realm]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms/finalizers,verbs=update
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=identityproviders,verbs=get;list;watch;create;update;patch;delete

func (r *RealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &identityv1.Realm{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("realm-controller")
	r.Controller = commonController.NewController(&realmHandler.HandlerRealm{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&identityv1.Realm{}).
		Watches(&identityv1.IdentityProvider{},
			handler.EnqueueRequestsFromMapFunc(r.mapIdpObjToRealm),
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
			RateLimiter:             workqueue.DefaultTypedItemBasedRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}

// mapIdpObjToRealm maps identity provider object to reconcile requests.
func (r *RealmReconciler) mapIdpObjToRealm(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	idp, ok := obj.(*identityv1.IdentityProvider)
	if !ok {
		logger.V(0).Info("object is not an IdentityProvider")
		return nil
	}

	list := &identityv1.RealmList{}
	err := r.Client.List(ctx, list, client.MatchingLabels{
		config.EnvironmentLabelKey: idp.Labels[config.EnvironmentLabelKey],
	})
	if err != nil {
		logger.Error(err, "failed to list Realms")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		if idp.UID == item.UID {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
	}

	return requests
}
