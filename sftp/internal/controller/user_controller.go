// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

//nolint:dupl // kubebuilder controller scaffolding is structurally identical across CRD types
package controller

import (
	"context"
	"fmt"

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
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	user_handler "github.com/telekom/controlplane/sftp/internal/handler/user"
	"github.com/telekom/controlplane/sftp/internal/service"
)

// UserReconciler reconciles a User object.
type UserReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	ServiceFactory service.Factory

	cc.Controller[*sftpv1.User]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users/finalizers,verbs=update
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=instances,verbs=get;list;watch

func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &sftpv1.User{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	userHandler, err := user_handler.New(r.ServiceFactory)
	if err != nil {
		return fmt.Errorf("creating user handler: %w", err)
	}

	r.Recorder = mgr.GetEventRecorderFor("user-controller")
	r.Controller = cc.NewController(userHandler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&sftpv1.User{}).
		Watches(&sftpv1.Instance{},
			handler.EnqueueRequestsFromMapFunc(r.MapInstanceToUsers),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *UserReconciler) MapInstanceToUsers(ctx context.Context, obj client.Object) []reconcile.Request {
	instance, ok := obj.(*sftpv1.Instance)
	if !ok {
		return nil
	}

	list := &sftpv1.UserList{}
	err := r.List(ctx, list, client.MatchingFields{sftpv1.IndexFieldSpecInstanceRef: commontypes.ObjectRefFromObject(instance).String()})
	if err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}
