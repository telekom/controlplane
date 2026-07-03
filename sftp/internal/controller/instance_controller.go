// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/labels"
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
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	instance_handler "github.com/telekom/controlplane/sftp/internal/handler/instance"
	"github.com/telekom/controlplane/sftp/internal/service"
)

// InstanceReconciler reconciles an Instance object.
type InstanceReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Recorder       record.EventRecorder
	ServiceFactory service.Factory

	cc.Controller[*sftpv1.Instance]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=instances/finalizers,verbs=update
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=sftpserviceconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=users/finalizers,verbs=update

func (r *InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &sftpv1.Instance{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("instance-controller")
	r.Controller = cc.NewController(&instance_handler.InstanceHandler{
		Client:         r.Client,
		ServiceFactory: r.ServiceFactory,
	}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&sftpv1.Instance{}).
		Watches(&sftpv1.SFTPServiceConfig{},
			handler.EnqueueRequestsFromMapFunc(r.MapSFTPServiceConfigToInstance),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&sftpv1.User{},
			handler.EnqueueRequestsFromMapFunc(r.MapUserToInstance),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

func (r *InstanceReconciler) MapSFTPServiceConfigToInstance(ctx context.Context, obj client.Object) []reconcile.Request {
	sftpServiceConfig, ok := obj.(*sftpv1.SFTPServiceConfig)
	if !ok {
		return nil
	}

	list := &sftpv1.InstanceList{}
	if err := r.List(ctx, list, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			sftpv1.SFTPServiceConfigNameKey:      sftpServiceConfig.Name,
			sftpv1.SFTPServiceConfigNamespaceKey: sftpServiceConfig.Namespace,
		}),
	}); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}

func (r *InstanceReconciler) MapUserToInstance(_ context.Context, obj client.Object) []reconcile.Request {
	user, ok := obj.(*sftpv1.User)
	if !ok || user.Spec.InstanceRef.IsEmpty() {
		return nil
	}

	return []reconcile.Request{{NamespacedName: user.Spec.InstanceRef.K8s()}}
}
