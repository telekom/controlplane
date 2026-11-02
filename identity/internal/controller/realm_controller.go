// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // Controllers intentionally mirror each other.
package controller

import (
	"context"
	"errors"

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

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	realmHandler "github.com/telekom/controlplane/identity/internal/handler/realm"
	"github.com/telekom/controlplane/identity/internal/leader"
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
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms/finalizers,verbs=update
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=identityproviders,verbs=get;list;watch;create;update;patch;delete

func (r *RealmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &identityv1.Realm{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RealmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.ClientFactory == nil {
		return errors.New("keycloak service factory is required")
	}
	r.Recorder = mgr.GetEventRecorderFor("realm-controller")
	reconciler := realmHandler.NewHandlerRealm(r.ClientFactory)
	reconciler.PeerReader = r.Client
	r.Controller = cc.NewController(reconciler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&identityv1.Realm{}, builder.WithPredicates(cc.Count("realm", cc.RoleFor))).
		Watches(&identityv1.Realm{},
			handler.EnqueueRequestsFromMapFunc(r.mapRealmObjToRealm),
			builder.WithPredicates(cc.Count("realm", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		Watches(&identityv1.IdentityProvider{},
			handler.EnqueueRequestsFromMapFunc(r.mapIdpObjToRealm),
			builder.WithPredicates(cc.Count("realm", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// mapIdpObjToRealm maps all realms sharing the provider's Keycloak admin URL.
func (r *RealmReconciler) mapIdpObjToRealm(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	idp, ok := obj.(*identityv1.IdentityProvider)
	if !ok {
		logger.V(0).Info("object is not an IdentityProvider")
		return nil
	}
	realms, err := leader.RealmsForAdminURL(ctx, r.Client, idp)
	if err != nil {
		logger.Error(err, "failed to list shared Realms")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(realms))
	for i := range realms {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&realms[i])})
	}
	return uniqueRequests(requests)
}

func (r *RealmReconciler) mapRealmObjToRealm(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	realm, ok := obj.(*identityv1.Realm)
	if !ok {
		logger.V(0).Info("object is not a Realm")
		return nil
	}

	peers, err := leader.RealmPeersForRealm(ctx, r.Client, realm)
	if err != nil {
		logger.Error(err, "failed to list Realm peers", "realm", client.ObjectKeyFromObject(realm))
		return nil
	}
	requests := make([]reconcile.Request, 0, len(peers))
	for i := range peers {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&peers[i])})
	}
	return uniqueRequests(requests)
}

func uniqueRequests(requests []reconcile.Request) []reconcile.Request {
	unique := make([]reconcile.Request, 0, len(requests))
	seen := make(map[client.ObjectKey]struct{}, len(requests))
	for _, request := range requests {
		if _, ok := seen[request.NamespacedName]; ok {
			continue
		}
		seen[request.NamespacedName] = struct{}{}
		unique = append(unique, request)
	}
	return unique
}
