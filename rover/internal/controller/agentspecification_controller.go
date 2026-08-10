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
	"sigs.k8s.io/controller-runtime/pkg/controller"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	rover "github.com/telekom/controlplane/rover/api/v1"
	agentspec_handler "github.com/telekom/controlplane/rover/internal/handler/agentspecification"
)

// AgentSpecificationReconciler reconciles an AgentSpecification object
type AgentSpecificationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.AgentSpecification]
}

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=agentspecifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=agentspecifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=agentspecifications/finalizers,verbs=update
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agentcards,verbs=get;list;watch;create;update;patch;delete

func (r *AgentSpecificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.AgentSpecification{})
}

func (r *AgentSpecificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("agentspecification-controller")
	r.Controller = cc.NewController(&agentspec_handler.AgentSpecificationHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&rover.AgentSpecification{}).
		Owns(&agenticv1.AgentCard{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
