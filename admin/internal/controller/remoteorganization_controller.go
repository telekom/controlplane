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
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	remoteorg_handler "github.com/telekom/controlplane/admin/internal/handler/remoteorganization"
)

// RemoteOrganizationReconciler reconciles a RemoteOrganization object
type RemoteOrganizationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controller.Controller[*adminv1.RemoteOrganization]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=remoteorganizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=remoteorganizations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=remoteorganizations/finalizers,verbs=update

func (r *RemoteOrganizationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &adminv1.RemoteOrganization{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteOrganizationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("remoteorganization-controller")
	r.Controller = controller.NewController(&remoteorg_handler.RemoteOrganizationHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&adminv1.RemoteOrganization{}).
		Complete(r)
}
