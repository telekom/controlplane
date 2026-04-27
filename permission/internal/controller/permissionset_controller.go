// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	"github.com/telekom/controlplane/permission/internal/handler/permissionset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// PermissionSetReconciler reconciles a PermissionSet object
type PermissionSetReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*permissionv1.PermissionSet]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=permission.cp.ei.telekom.de,resources=permissionsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=permission.cp.ei.telekom.de,resources=permissionsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=permission.cp.ei.telekom.de,resources=permissionsets/finalizers,verbs=update

// +kubebuilder:rbac:groups=pcp.ei.telekom.de,resources=permissionsets,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get

func (r *PermissionSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &permissionv1.PermissionSet{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *PermissionSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("permissionset-controller")
	r.Controller = cc.NewController(&permissionset.PermissionSetHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&permissionv1.PermissionSet{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
