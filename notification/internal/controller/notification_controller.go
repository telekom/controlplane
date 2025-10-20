// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/notification/internal/sender"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
	"github.com/telekom/controlplane/notification/internal/sender/adapter/mail"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/telekom/controlplane/notification/internal/handler"

	adapterconfig "github.com/telekom/controlplane/notification/internal/config"
)

// NotificationReconciler reconciles a Notification object
type NotificationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	cc.Controller[*notificationv1.Notification]

	NotificationSender sender.NotificationSender
}

func NewNotificationReconcilerWithSenderConfig(
	client client.Client,
	scheme *runtime.Scheme,
	emailConfig *adapterconfig.EmailAdapterConfig,
) *NotificationReconciler {

	// initialize the notification sender with all adapters so they can be reused
	notificationSender := &sender.AdapterSender{
		MailAdapter: &mail.EmailAdapter{
			AdapterConfig: emailConfig,
		},
		ChatAdapter:     adapter.NewMsTeamsAdapter(),
		CallbackAdapter: &adapter.WebhookAdapter{},
	}

	return &NotificationReconciler{
		Client:             client,
		Scheme:             scheme,
		NotificationSender: notificationSender,
	}
}

// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=notification.cp.ei.telekom.de,resources=notifications/finalizers,verbs=update

func (r *NotificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Controller.Reconcile(ctx, req, &notificationv1.Notification{})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NotificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("notification-controller")

	//// initialize the notification sender with all adapters so they can be reused
	//notificationSender := sender.AdapterSender{
	//	MailAdapter: &adapter.EmailAdapter{
	//		SMTPHost: emailConfig.SMTPHost,
	//		SMTPPort: emailConfig.SMTPPort,
	//	},
	//	ChatAdapter:     adapter.NewMsTeamsAdapter(),
	//	CallbackAdapter: &adapter.WebhookAdapter{},
	//}

	notificationHandler := &handler.NotificationHandler{
		NotificationSender: r.NotificationSender,
	}

	r.Controller = cc.NewController(notificationHandler, r.Client, r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&notificationv1.Notification{}).
		Named("notification").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: cconfig.MaxConcurrentReconciles,
			RateLimiter:             cc.NewRateLimiter(),
		}).
		Complete(r)
}
