// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // Single-resource controller scaffolds are intentionally kept parallel for clarity.
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
	mcpspec_handler "github.com/telekom/controlplane/rover/internal/handler/mcpspecification"
)

// McpSpecificationReconciler reconciles a McpSpecification object
type McpSpecificationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.McpSpecification]
}

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=mcpspecifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=mcpspecifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=mcpspecifications/finalizers,verbs=update
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticservers,verbs=get;list;watch;create;update;patch;delete

func (r *McpSpecificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.McpSpecification{})
}

func (r *McpSpecificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("mcpspecification-controller")
	r.Controller = cc.NewController(&mcpspec_handler.McpSpecificationHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&rover.McpSpecification{}).
		Owns(&agenticv1.AgenticServer{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
