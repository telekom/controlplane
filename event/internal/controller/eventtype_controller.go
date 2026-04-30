// Copyright 2025 Deutsche Telekom IT GmbH
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
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventtype"
)

// EventTypeReconciler reconciles a EventType object
type EventTypeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*eventv1.EventType]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventtypes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventtypes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventtypes/finalizers,verbs=update

func (r *EventTypeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &eventv1.EventType{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventTypeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("eventtype-controller")
	r.Controller = cc.NewController(&eventtype.EventTypeHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&eventv1.EventType{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
