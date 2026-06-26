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

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	filev1 "github.com/telekom/controlplane/file/api/v1"
	filesubscription_handler "github.com/telekom/controlplane/file/internal/handler/filesubscription"
	"github.com/telekom/controlplane/file/internal/index"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// FileSubscriptionReconciler reconciles a FileSubscription object.
type FileSubscriptionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*filev1.FileSubscription]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filesubscriptions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filesubscriptions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filesubscriptions/finalizers,verbs=update
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=filetypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=file.cp.ei.telekom.de,resources=fileexposures,verbs=get;list;watch
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvalrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=approval.cp.ei.telekom.de,resources=approvals,verbs=get;list;watch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users/status,verbs=get

func (r *FileSubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &filev1.FileSubscription{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *FileSubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("filesubscription-controller")
	r.Controller = cc.NewController(&filesubscription_handler.FileSubscriptionHandler{}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&filev1.FileSubscription{}).
		Owns(&approvalv1.ApprovalRequest{}).
		Owns(&approvalv1.Approval{}).
		Owns(&sftpv1.User{}).
		Watches(&filev1.FileType{},
			handler.EnqueueRequestsFromMapFunc(r.MapFileTypeToFileSubscription),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *FileSubscriptionReconciler) MapFileTypeToFileSubscription(ctx context.Context, obj client.Object) []reconcile.Request {
	fileType, ok := obj.(*filev1.FileType)
	if !ok {
		return nil
	}

	list := &filev1.FileSubscriptionList{}
	err := r.List(ctx, list,
		client.MatchingFields{index.FieldSpecFileTypeOnSubscription: fileType.Name},
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
