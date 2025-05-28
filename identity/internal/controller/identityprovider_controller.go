// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	commonController "github.com/telekom/controlplane/common/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	identityproviderHandler "github.com/telekom/controlplane/identity/internal/handler/identityprovider"
)

// IdentityProviderReconciler reconciles a IdentityProvider object
type IdentityProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	commonController.Controller[*identityv1.IdentityProvider]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=identityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=identityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=identityproviders/finalizers,verbs=update

func (r *IdentityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &identityv1.IdentityProvider{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *IdentityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("identityprovider-controller")
	r.Controller = commonController.NewController(&identityproviderHandler.HandlerIdentityProvider{}, r.Client, r.Recorder)

	// TODO CreateOrUpdate realms in keycloak

	return ctrl.NewControllerManagedBy(mgr).
		For(&identityv1.IdentityProvider{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,
			RateLimiter:             workqueue.DefaultTypedItemBasedRateLimiter[reconcile.Request](),
		}).
		Complete(r)
}
