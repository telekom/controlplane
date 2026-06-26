// Copyright 2025 Deutsche Telekom IT GmbH
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

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	zoneserviceconfig_handler "github.com/telekom/controlplane/file/internal/handler/zoneserviceconfig"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// ZoneServiceConfigReconciler reconciles a ZoneServiceConfig object.
type ZoneServiceConfigReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*filev1.ZoneServiceConfig]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=zoneserviceconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=zoneserviceconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=zoneserviceconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=admin.cp.ei.telekom.de,resources=zones/status,verbs=get
// +kubebuilder:rbac:groups=identity.cp.ei.telekom.de,resources=clients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=sftpserviceconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=sftpserviceconfigs/status,verbs=get
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.cp.ei.telekom.de,resources=routes/status,verbs=get

func (r *ZoneServiceConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &filev1.ZoneServiceConfig{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneServiceConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("zoneserviceconfig-controller")
	r.Controller = cc.NewController(&zoneserviceconfig_handler.ZoneServiceConfigHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&filev1.ZoneServiceConfig{}).
		Owns(&identityv1.Client{}).
		Owns(&sftpv1.SFTPServiceConfig{}).
		Watches(&adminv1.Zone{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneToZoneServiceConfig),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *ZoneServiceConfigReconciler) MapZoneToZoneServiceConfig(ctx context.Context, obj client.Object) []reconcile.Request {
	zone, ok := obj.(*adminv1.Zone)
	if !ok {
		return nil
	}

	return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(zone)}}
}
