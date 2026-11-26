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

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/apichangelog"
	cc "github.com/telekom/controlplane/common/pkg/controller"
)

// ApiChangelogReconciler reconciles a ApiChangelog object
type ApiChangelogReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*apiv1.ApiChangelog]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apichangelogs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apichangelogs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apichangelogs/finalizers,verbs=update

func (r *ApiChangelogReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &apiv1.ApiChangelog{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApiChangelogReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("apichangelog-controller")
	r.Controller = cc.NewController(&apichangelog.ApiChangelogHandler{Client: r.Client}, r.Client, r.Recorder)

	return setupBasePathController(mgr, basePathKind[*apiv1.ApiChangelog, *apiv1.ApiChangelogList]{
		name:     "apichangelog",
		newObj:   func() *apiv1.ApiChangelog { return &apiv1.ApiChangelog{} },
		newList:  func() *apiv1.ApiChangelogList { return &apiv1.ApiChangelogList{} },
		basePath: func(o *apiv1.ApiChangelog) string { return o.Spec.BasePath },
	}, r)
}
