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
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	clientHandler "github.com/telekom/controlplane/identity/internal/handler/client"
	"github.com/telekom/controlplane/identity/internal/leader"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
)

// ClientReconciler reconciles a Client object
type ClientReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientFactory keycloak.ServiceFactory

	cc.Controller[*identityv1.Client]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients/finalizers,verbs=update
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms,verbs=get;list;watch;create;update;patch;delete

func (r *ClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &identityv1.Client{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.ClientFactory == nil {
		return errors.New("keycloak service factory is required")
	}
	r.Recorder = mgr.GetEventRecorderFor("client-controller")
	reconciler := clientHandler.NewHandlerClient(r.ClientFactory)
	reconciler.PeerReader = r.Client
	r.Controller = cc.NewController(reconciler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&identityv1.Client{}, builder.WithPredicates(cc.Count("client", cc.RoleFor))).
		Watches(&identityv1.Client{},
			handler.EnqueueRequestsFromMapFunc(r.mapClientObjToIdentityClient),
			builder.WithPredicates(cc.Count("client", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		Watches(&identityv1.Realm{},
			handler.EnqueueRequestsFromMapFunc(r.mapRealmObjToIdentityClient),
			builder.WithPredicates(cc.Count("client", cc.RoleWatches, predicate.ResourceVersionChangedPredicate{}))).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// mapRealmObjToIdentityClient maps a Realm change to clients in all shared realms.
func (r *ClientReconciler) mapRealmObjToIdentityClient(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	realm, ok := obj.(*identityv1.Realm)
	if !ok {
		logger.V(0).Info("object is not a Realm")
		return nil
	}
	realms, err := leader.RealmPeersForRealm(ctx, r.Client, realm)
	if err != nil {
		logger.Error(err, "failed to list shared Realm peers", "realm", client.ObjectKeyFromObject(realm))
		return nil
	}
	requests := make([]reconcile.Request, 0)
	seen := make(map[client.ObjectKey]struct{})
	for i := range realms {
		list := &identityv1.ClientList{}
		ref := types.ObjectRefFromObject(&realms[i]).String()
		if err := r.List(ctx, list, client.MatchingFields{IndexFieldSpecRealm: ref}); err != nil {
			logger.Error(err, "failed to list Clients for Realm", "realm", ref)
			continue
		}
		for j := range list.Items {
			key := client.ObjectKeyFromObject(&list.Items[j])
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			requests = append(requests, reconcile.Request{NamespacedName: key})
		}
	}
	return requests
}

func (r *ClientReconciler) mapClientObjToIdentityClient(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)
	identityClient, ok := obj.(*identityv1.Client)
	if !ok || identityClient.Spec.Realm == nil {
		logger.V(0).Info("object is not a Client with a Realm")
		return nil
	}
	realm := &identityv1.Realm{}
	if err := r.Get(ctx, identityClient.Spec.Realm.K8s(), realm); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to get Client Realm", "client", client.ObjectKeyFromObject(identityClient))
		}
		return nil
	}
	realms, err := leader.RealmPeersForRealm(ctx, r.Client, realm)
	if err != nil {
		logger.Error(err, "failed to list shared Realm peers", "realm", client.ObjectKeyFromObject(realm))
		return nil
	}
	peers, err := leader.ClientsForRealmGroup(ctx, r.Client, realms, identityClient)
	if err != nil {
		logger.Error(err, "failed to list Client peers", "client", client.ObjectKeyFromObject(identityClient))
		return nil
	}
	requests := make([]reconcile.Request, 0, len(peers))
	for i := range peers {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&peers[i])})
	}
	return requests
}
