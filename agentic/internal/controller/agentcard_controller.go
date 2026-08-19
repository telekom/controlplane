// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // parallel structure with mcpserver_controller.go; operates on different types
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
	"github.com/telekom/controlplane/agentic/internal/handler/agentcard"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
)

// AgentCardReconciler reconciles an AgentCard object
type AgentCardReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*agenticv1.AgentCard]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agentcards,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agentcards/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agentic.cp.ei.telekom.de,resources=agentcards/finalizers,verbs=update

func (r *AgentCardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &agenticv1.AgentCard{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentCardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("agentcard-controller")
	r.Controller = cc.NewController(&agentcard.AgentCardHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&agenticv1.AgentCard{}).
		Watches(&agenticv1.AgentCard{},
			handler.EnqueueRequestsFromMapFunc(r.MapAgentCardToAgentCard),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// MapAgentCardToAgentCard enqueues other AgentCards with the same basePath
// when any AgentCard changes or is deleted.
//
//nolint:dupl // parallel structure with MapMcpServerToMcpServer; operates on different types
func (r *AgentCardReconciler) MapAgentCardToAgentCard(ctx context.Context, obj client.Object) []reconcile.Request {
	card, ok := obj.(*agenticv1.AgentCard)
	if !ok {
		return nil
	}

	list := &agenticv1.AgentCardList{}
	if err := r.List(ctx, list, client.MatchingLabels{
		cconfig.EnvironmentLabelKey:     card.Labels[cconfig.EnvironmentLabelKey],
		agenticv1.AgentBasePathLabelKey: labelutil.NormalizeLabelValue(card.Spec.BasePath),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for i := range list.Items {
		if list.Items[i].UID == card.UID {
			continue
		}
		if list.Items[i].Spec.BasePath == card.Spec.BasePath {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			})
		}
	}
	return reqs
}
