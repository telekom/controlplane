// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/internal/sender"
	"github.com/telekom/controlplane/notification/internal/sender/adapter/mail"
	"github.com/telekom/controlplane/notification/internal/sender/adapter/msteams"
	"github.com/telekom/controlplane/notification/internal/sender/adapter/webhook"
	"github.com/telekom/controlplane/notification/internal/templatecache"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"sigs.k8s.io/controller-runtime/pkg/handler"

	notificationhandler "github.com/telekom/controlplane/notification/internal/handler"

	notificationsconfig "github.com/telekom/controlplane/notification/internal/config"
)

// NotificationReconciler reconciles a Notification object
type NotificationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*notificationv1.Notification]

	NotificationSender sender.NotificationSender

	HousekeepingConfig notificationsconfig.NotificationHousekeepingConfig

	TemplateCache *templatecache.TemplateCache
}

func NewNotificationReconcilerWithConfig(
	client client.Client,
	scheme *runtime.Scheme,
	emailConfig *notificationsconfig.EmailAdapterConfig,
	housekeepingConfig *notificationsconfig.NotificationHousekeepingConfig,
	TemplateCache *templatecache.TemplateCache,
) *NotificationReconciler {

	// initialize the notification sender with all adapters so they can be reused
	notificationSender := &sender.AdapterSender{
		MailAdapter: &mail.EmailAdapter{
			AdapterConfig: emailConfig,
		},
		ChatAdapter:     msteams.NewMsTeamsAdapter(),
		CallbackAdapter: &webhook.WebhookAdapter{},
	}

	return &NotificationReconciler{
		Client:             client,
		Scheme:             scheme,
		NotificationSender: notificationSender,
		HousekeepingConfig: *housekeepingConfig,
		TemplateCache:      TemplateCache,
	}
}

// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications/finalizers,verbs=update

// Notifications are created in team namespaces. The controller needs permission to create events in all namespaces where notifications exist.
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *NotificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &notificationv1.Notification{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NotificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("notification-controller")

	notificationHandler := &notificationhandler.NotificationHandler{
		NotificationSender: r.NotificationSender,
		HousekeepingConfig: r.HousekeepingConfig,
		TemplateCache:      r.TemplateCache,
	}

	r.Controller = cc.NewController(notificationHandler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationv1.Notification{}).
		Watches(&notificationv1.NotificationChannel{},
			handler.EnqueueRequestsFromMapFunc(r.MapChannelToNotification),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(&notificationv1.NotificationTemplate{},
			handler.EnqueueRequestsFromMapFunc(r.MapTemplateToNotification),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("notification").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
