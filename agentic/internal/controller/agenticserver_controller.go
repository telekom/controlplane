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
	"github.com/telekom/controlplane/agentic/internal/handler/agenticserver"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// AgenticServerReconciler reconciles a AgenticServer object
type AgenticServerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*agenticv1.AgenticServer]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agenticservers/finalizers,verbs=update

func (r *AgenticServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &agenticv1.AgenticServer{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgenticServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("agenticserver-controller")
	r.Controller = cc.NewController(&agenticserver.AgenticServerHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticv1.AgenticServer{}).
		Watches(&agenticv1.AgenticServer{},
			handler.EnqueueRequestsFromMapFunc(r.MapAgenticServerToAgenticServer),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapAgenticServerToAgenticServer enqueues other AgenticServers with the same basePath
// when any AgenticServer changes or is deleted.
//
//nolint:dupl // parallel structure with MapAgenticExposureToAgenticExposure; operates on different types
func (r *AgenticServerReconciler) MapAgenticServerToAgenticServer(ctx context.Context, obj client.Object) []reconcile.Request {
	server, ok := obj.(*agenticv1.AgenticServer)
	if !ok {
		return nil
	}

	list := &agenticv1.AgenticServerList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:       server.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.AgenticBasePathLabelKey: labelutil.NormalizeLabelValue(server.Spec.BasePath),
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
