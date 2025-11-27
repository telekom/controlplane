// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"github.com/telekom/controlplane/notification/internal/templatecache"
	texttemplate "text/template"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/internal/handler"
)

// NotificationTemplateReconciler reconciles a NotificationTemplate object
type NotificationTemplateReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*notificationv1.NotificationTemplate]

	Cache           *templatecache.TemplateCache
	CustomFunctions texttemplate.FuncMap
}

// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notificationtemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notificationtemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notificationtemplates/finalizers,verbs=update

func (r *NotificationTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &notificationv1.NotificationTemplate{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NotificationTemplateReconciler) SetupWithManager(mgr ctrl.Manager, cache *templatecache.TemplateCache) error {
	r.Recorder = mgr.GetEventRecorderFor("notificationtemplate-controller")
	r.Controller = cc.NewController(&handler.NotificationTemplateHandler{
		Cache:           cache,
		CustomFunctions: getCustomTemplateFunctions(),
	}, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationv1.NotificationTemplate{}).
		Named("notificationtemplate").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}

// place custom functions here
// see template_renderer_test for a simple example
func getCustomTemplateFunctions() texttemplate.FuncMap {
	return texttemplate.FuncMap{}
}
