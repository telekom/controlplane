// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
	sftpserviceconfig_handler "github.com/telekom/controlplane/sftp/internal/handler/sftpserviceconfig"
	"github.com/telekom/controlplane/sftp/internal/service"
)

// SFTPServiceConfigReconciler reconciles an SFTPServiceConfig object
type SFTPServiceConfigReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	ClientManager service.ClientManager

	cc.Controller[*sftpv1.SFTPServiceConfig]
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=sftpserviceconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=sftpserviceconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sftp.cp.ei.telekom.de,resources=sftpserviceconfigs/finalizers,verbs=update

func (r *SFTPServiceConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &sftpv1.SFTPServiceConfig{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *SFTPServiceConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	sftpserviceconfigHandler, err := sftpserviceconfig_handler.New(r.ClientManager)
	if err != nil {
		return fmt.Errorf("creating SFTPServiceConfig handler: %w", err)
	}
	r.Recorder = mgr.GetEventRecorderFor("sftpserviceconfig-controller")
	r.Controller = cc.NewController(sftpserviceconfigHandler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&sftpv1.SFTPServiceConfig{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
