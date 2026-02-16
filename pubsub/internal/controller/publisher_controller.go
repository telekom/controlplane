// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	"github.com/telekom/controlplane/pubsub/internal/handler/publisher"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// PublisherReconciler reconciles a Publisher object
type PublisherReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*pubsubv1.Publisher]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=publishers/finalizers,verbs=update
// +kubebuilder:rbac:groups=pubsub.cp.ei.telekom.de,resources=eventstores,verbs=get

func (r *PublisherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &pubsubv1.Publisher{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *PublisherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("publisher-controller")
	r.Controller = cc.NewController(&publisher.PublisherHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&pubsubv1.Publisher{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
