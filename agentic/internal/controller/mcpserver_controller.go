// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/mcpserver"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// McpServerReconciler reconciles a McpServer object
type McpServerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*agenticv1.McpServer]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=mcpservers/finalizers,verbs=update

func (r *McpServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &agenticv1.McpServer{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *McpServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("mcpserver-controller")
	r.Controller = cc.NewController(&mcpserver.McpServerHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticv1.McpServer{}).
		Watches(&agenticv1.McpServer{},
			handler.EnqueueRequestsFromMapFunc(r.MapMcpServerToMcpServer),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapMcpServerToMcpServer enqueues other McpServers with the same basePath
// when any McpServer changes or is deleted.
//
//nolint:dupl // parallel structure with MapMcpExposureToMcpExposure; operates on different types
func (r *McpServerReconciler) MapMcpServerToMcpServer(ctx context.Context, obj client.Object) []reconcile.Request {
	server, ok := obj.(*agenticv1.McpServer)
	if !ok {
		return nil
	}

	list := &agenticv1.McpServerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:   server.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.McpBasePathLabelKey: labelutil.NormalizeLabelValue(server.Spec.BasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].UID == server.UID {
			continue
		}
		if list.Items[i].Spec.BasePath == server.Spec.BasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}
