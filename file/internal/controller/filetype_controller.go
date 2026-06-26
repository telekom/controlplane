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
	filetype_handler "github.com/telekom/controlplane/file/internal/handler/filetype"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// FileTypeReconciler reconciles a FileType object.
type FileTypeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*filev1.FileType]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filetypes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filetypes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filetypes/finalizers,verbs=update
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=fileexposures,verbs=get;list;watch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=zoneserviceconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users,verbs=get;list;watch;create;update;patch;delete

func (r *FileTypeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &filev1.FileType{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *FileTypeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("filetype-controller")
	r.Controller = cc.NewController(&filetype_handler.FileTypeHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&filev1.FileType{}).
		Owns(&sftpv1.User{}).
		Watches(&filev1.FileExposure{},
			handler.EnqueueRequestsFromMapFunc(r.MapFileExposureToFileType),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *FileTypeReconciler) MapFileExposureToFileType(ctx context.Context, obj client.Object) []reconcile.Request {
	exposure, ok := obj.(*filev1.FileExposure)
	if !ok {
		return nil
	}

	key := client.ObjectKeyFromObject(obj)
	key.Name = exposure.Spec.FileType
	return []reconcile.Request{{
		NamespacedName: key,
	}}
}
