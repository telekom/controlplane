// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/apiroadmap"
	cc "github.com/telekom/controlplane/common/pkg/controller"
)

// ApiRoadmapReconciler reconciles a ApiRoadmap object
type ApiRoadmapReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*apiv1.ApiRoadmap]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiroadmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiroadmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiroadmaps/finalizers,verbs=update

func (r *ApiRoadmapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &apiv1.ApiRoadmap{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApiRoadmapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("apiroadmap-controller")
	r.Controller = cc.NewController(&apiroadmap.ApiRoadmapHandler{Client: r.Client}, r.Client, r.Recorder)

	return setupBasePathController(mgr, apiRoadmapKind, r)
}
