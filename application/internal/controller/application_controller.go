// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	record "k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/application/internal/handler/application"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	"github.com/telekom/controlplane/common/pkg/controller/index"
	gateway "github.com/telekom/controlplane/gateway/api/v1"
	identity "github.com/telekom/controlplane/identity/api/v1"
)

// ApplicationReconciler reconciles a Application object
type ApplicationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*applicationv1.Application]
}

// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications/finalizers,verbs=update

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=realms,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=consumers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch

func (r *ApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &applicationv1.Application{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("application-controller")
	r.Controller = cc.NewController(&application.ApplicationHandler{}, r.Client, r.Recorder)

	ctx := context.TODO()

	err := index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &identity.Client{})
	if err != nil {
		return err
	}

	err = index.SetOwnerIndex(ctx, mgr.GetFieldIndexer(), &gateway.Consumer{})
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&applicationv1.Application{}).
		Owns(&identity.Client{}).
		Owns(&gateway.Consumer{}).
		Complete(r)
}
