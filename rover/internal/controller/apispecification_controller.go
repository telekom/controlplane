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

	apispec_handler "github.com/telekom/controlplane/rover/internal/handler/apispecification"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

// ApiSpecificationReconciler reconciles a ApiSpecification object
type ApiSpecificationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*rover.ApiSpecification]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=apispecifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=apispecifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rover.cp.ei.telekom.de,resources=apispecifications/finalizers,verbs=update
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch

func (r *ApiSpecificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &rover.ApiSpecification{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApiSpecificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("apispecification-controller")

	h := &apispec_handler.ApiSpecificationHandler{
		ListZones: func(ctx context.Context, environment string) (*adminv1.ZoneList, error) {
			c := cclient.ClientFromContextOrDie(ctx)
			list := &adminv1.ZoneList{}
			err := c.List(ctx, list, client.InNamespace(environment))
			return list, err
		},
	}

	r.Controller = cc.NewController(h, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&rover.ApiSpecification{}).
		Owns(&apiapi.Api{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
