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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	roadmap_handler "github.com/telekom/controlplane/rover/internal/handler/roadmap"

	rover "github.com/telekom/controlplane/rover/api/v1"
)

// RoadmapReconciler reconciles a Roadmap object
type RoadmapReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.Roadmap]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=roadmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=roadmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=roadmaps/finalizers,verbs=update

func (r *RoadmapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.Roadmap{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoadmapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("roadmap-controller")
	r.Controller = cc.NewController(&roadmap_handler.RoadmapHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&rover.Roadmap{}).
		// Note: No .Owns() call here - Roadmap doesn't create any owned resources
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
