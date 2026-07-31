// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
)

// SpectreApplicationReconciler reconciles a SpectreApplication object
type SpectreApplicationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*spectrev1.SpectreApplication]
}

// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=spectreapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=spectreapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=spectre.cp.ei.telekom.de,resources=spectreapplications/finalizers,verbs=update

func (r *SpectreApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &spectrev1.SpectreApplication{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *SpectreApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("spectreapplication-controller")
	r.Controller = cc.NewController(&handler.SpectreApplicationHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&spectrev1.SpectreApplication{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Named("spectreapplication").
		Complete(r)
}
