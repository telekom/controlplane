// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	handler "github.com/telekom/controlplane/organization/internal/handler/group"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// GroupReconciler reconciles a Group object
type GroupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*organizationv1.Group]
}

// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=groups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=groups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=groups/finalizers,verbs=update

func (r *GroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &organizationv1.Group{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *GroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("group-controller")
	r.Controller = cc.NewController(&handler.GroupHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&organizationv1.Group{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
