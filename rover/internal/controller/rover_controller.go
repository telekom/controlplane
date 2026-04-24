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

	rover_handler "github.com/telekom/controlplane/rover/internal/handler/rover"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	application "github.com/telekom/controlplane/application/api/v1"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	permissionv1 "github.com/telekom/controlplane/permission/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

// RoverReconciler reconciles a Rover object
type RoverReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.Rover]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=rovers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=rovers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=rovers/finalizers,verbs=update

// +kubebuilder:rbac:groups=organization.cp.ei.telekom.de,resources=teams,verbs=get;list;watch

// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apisubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apiexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apicategories,verbs=get;list;watch

// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=event.cp.ei.telekom.de,resources=eventsubscriptions,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=permission.cp.ei.telekom.de,resources=permissionsets,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch;create;update;patch;delete

func (r *RoverReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.Rover{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("rover-controller")
	r.Controller = cc.NewController(&rover_handler.RoverHandler{}, r.Client, r.Recorder)

	b := ctrl.NewControllerManagedBy(mgr).
		For(&rover.Rover{}).
		Owns(&apiapi.ApiSubscription{}).
		Owns(&apiapi.ApiExposure{}).
		Owns(&application.Application{})

	if cconfig.FeaturePubSub.IsEnabled() {
		b = b.Owns(&eventv1.EventExposure{}).
			Owns(&eventv1.EventSubscription{})
	}

	if cconfig.FeaturePermission.IsEnabled() {
		b = b.Owns(&permissionv1.PermissionSet{})
	}

	return b.WithOptions(controller.Options{
		MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
		RateLimiter:             cc.NewRateLimiter(),
	}).
		Complete(r)
}
