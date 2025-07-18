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

	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/remoteapisubscription"
	"github.com/telekom/controlplane/api/internal/handler/remoteapisubscription/syncer"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
)

// RemoteApiSubscriptionReconciler reconciles a RemoteApiSubscription object
type RemoteApiSubscriptionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*apiapi.RemoteApiSubscription]
}

// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=remoteapisubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=remoteapisubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=remoteapisubscriptions/finalizers,verbs=update
// +kubebuilder:rbac:groups=api.cp.ei.telekom.de,resources=apisubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=application.cp.ei.telekom.de,resources=applications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=remoteorganizations,verbs=get;list;watch

func (r *RemoteApiSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, new(apiapi.RemoteApiSubscription))
}

// SetupWithManager sets up the controller with the Manager.
func (r *RemoteApiSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager, syncerFactory syncer.SyncerClientFactory[*apiapi.RemoteApiSubscription]) error {
	r.Recorder = mgr.GetEventRecorderFor("remoteapisubscription-controller")
	handler := &remoteapisubscription.RemoteApiSubscriptionHandler{
		SyncerFactory: syncerFactory,
	}

	r.Controller = cc.NewController(handler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&apiapi.RemoteApiSubscription{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Owns(&apiapi.ApiSubscription{}).
		Owns(&applicationapi.Application{}).
		// Watch Routes
		Complete(r)
}
