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

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	fileexposure_handler "github.com/telekom/controlplane/file/internal/handler/fileexposure"
	"github.com/telekom/controlplane/file/internal/index"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// FileExposureReconciler reconciles a FileExposure object.
type FileExposureReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*filev1.FileExposure]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=fileexposures,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=fileexposures/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=fileexposures/finalizers,verbs=update
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filetypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=zoneserviceconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=instances,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users/status,verbs=get

func (r *FileExposureReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &filev1.FileExposure{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *FileExposureReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("fileexposure-controller")
	r.Controller = cc.NewController(&fileexposure_handler.FileExposureHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&filev1.FileExposure{}).
		Owns(&sftpv1.Instance{}).
		Owns(&sftpv1.User{}).
		Watches(&filev1.FileType{},
			handler.EnqueueRequestsFromMapFunc(r.MapFileTypeToFileExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&filev1.ZoneServiceConfig{},
			handler.EnqueueRequestsFromMapFunc(r.MapZoneServiceConfigToFileExposure),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *FileExposureReconciler) MapFileTypeToFileExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	fileType, ok := obj.(*filev1.FileType)
	if !ok {
		return nil
	}

	list := &filev1.FileExposureList{}
	err := r.List(ctx, list,
		client.InNamespace(fileType.Namespace),
		client.MatchingFields{index.FieldSpecFileTypeOnExposure: fileType.Name},
	)
	if err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}

func (r *FileExposureReconciler) MapZoneServiceConfigToFileExposure(ctx context.Context, obj client.Object) []reconcile.Request {
	zoneServiceConfig, ok := obj.(*filev1.ZoneServiceConfig)
	if !ok {
		return nil
	}

	list := &filev1.FileExposureList{}
	zoneKey := zoneServiceConfig.Namespace + "/" + zoneServiceConfig.Name
	if err := r.List(ctx, list,
		client.InNamespace(zoneServiceConfig.Namespace),
		client.MatchingFields{index.FieldSpecZoneOnExposure: zoneKey},
	); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}
