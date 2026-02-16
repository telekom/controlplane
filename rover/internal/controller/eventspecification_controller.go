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

	eventspec_handler "github.com/telekom/controlplane/rover/internal/handler/eventspecification"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

// EventSpecificationReconciler reconciles an EventSpecification object
type EventSpecificationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.EventSpecification]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=eventspecifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=eventspecifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=eventspecifications/finalizers,verbs=update
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventtypes,verbs=get;list;watch;create;update;patch;delete

func (r *EventSpecificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.EventSpecification{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventSpecificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("eventspecification-controller")
	r.Controller = cc.NewController(&eventspec_handler.EventSpecificationHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&rover.EventSpecification{}).
		Owns(&eventv1.EventType{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
