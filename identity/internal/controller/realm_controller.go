// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/types"
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

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	realmHandler "github.com/telekom/controlplane/identity/internal/handler/realm"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

// RealmReconciler reconciles a Realm object
type RealmReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientFactory keycloak.ServiceFactory

	cc.Controller[*identityv1.Realm]
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

	factory := r.ClientFactory
	if factory == nil {
		factory = keycloak.NewServiceFactory()
	}
	r.Controller = cc.NewController(realmHandler.NewHandlerRealm(factory), r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&identityv1.Realm{}).
		Watches(&identityv1.IdentityProvider{},
			handler.EnqueueRequestsFromMapFunc(r.mapIdpObjToRealm),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// mapIdpObjToRealm maps identity provider object to reconcile requests.
// Uses a field index on spec.identityProvider for efficient lookup instead
// of listing all Realms in the environment and filtering in-memory.
func (r *RealmReconciler) mapIdpObjToRealm(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	idp, ok := obj.(*identityv1.IdentityProvider)
	if !ok {
		logger.V(0).Info("object is not an IdentityProvider")
		return nil
	}
	if idp.Labels == nil {
		return nil
	}

	listOpts := []client.ListOption{
		client.MatchingFields{
			IndexFieldSpecIdentityProvider: types.ObjectRefFromObject(idp).String(),
		},
		client.MatchingLabels{
			cconfig.EnvironmentLabelKey: idp.Labels[cconfig.EnvironmentLabelKey],
		},
	}

	list := &identityv1.RealmList{}
	if err := r.Client.List(ctx, list, listOpts...); err != nil {
		logger.Error(err, "failed to list Realms")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(list.Items))
	for _, item := range list.Items {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&item)})
	}

	return requests
}
