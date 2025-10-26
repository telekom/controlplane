// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/notification/internal/sender/adapter/msteams"
	"github.com/telekom/controlplane/notification/internal/sender/adapter/webhook"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"

	"github.com/telekom/controlplane/notification/internal/sender"
	"github.com/telekom/controlplane/notification/internal/sender/adapter/mail"

	cconfig "github.com/telekom/controlplane/common/pkg/config"
	cc "github.com/telekom/controlplane/common/pkg/controller"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/controller-runtime/pkg/handler"

	notificationhandler "github.com/telekom/controlplane/notification/internal/handler"

	adapterconfig "github.com/telekom/controlplane/notification/internal/config"
)

const (
	IndexFieldSpecChannelRefs   = "spec.channelRefs"
	IndexFieldSpecTemplateNames = "spec.templateNames"
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
		ChatAdapter:     msteams.NewMsTeamsAdapter(),
		CallbackAdapter: &webhook.WebhookAdapter{},
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
	// Index Notifications by each channel reference
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&notificationv1.Notification{},
		IndexFieldSpecChannelRefs, // this is a *virtual* field name for your index
		func(obj client.Object) []string {
			n := obj.(*notificationv1.Notification)
			var keys []string
			for _, ch := range n.Spec.Channels {
				// We'll use "namespace/name" as the key
				keys = append(keys, fmt.Sprintf("%s/%s", ch.Namespace, ch.Name))
			}
			return keys
		},
	); err != nil {
		return errors.Wrapf(err, "Failed to create index for Notification for %q", IndexFieldSpecChannelRefs)
	}

	// Index Notifications by each template
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&notificationv1.Notification{},
		IndexFieldSpecTemplateNames, // this is a *virtual* field name for your index
		func(obj client.Object) []string {
			n := obj.(*notificationv1.Notification)
			var keys []string
			for _, ch := range n.Spec.Channels {

				parts := strings.Split(ch.Name, "--")
				channelType := parts[len(parts)-1]

				// We'll construct the template name as the key
				keys = append(keys, fmt.Sprintf("%s--%s", n.Spec.Purpose, channelType))
			}
			return keys
		},
	); err != nil {
		return errors.Wrapf(err, "Failed to create index for Notification for %q", IndexFieldSpecTemplateNames)
	}

	r.Recorder = mgr.GetEventRecorderFor("notification-controller")

	notificationHandler := &notificationhandler.NotificationHandler{
		NotificationSender: r.NotificationSender,
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

func (r *NotificationReconciler) MapChannelToNotification(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	ch := obj.(*notificationv1.NotificationChannel)
	channelKey := fmt.Sprintf("%s/%s", ch.Namespace, ch.Name)

	var notifications notificationv1.NotificationList
	if err := r.Client.List(ctx, &notifications, client.MatchingFields{IndexFieldSpecChannelRefs: channelKey}); err != nil {
		log.Error(err, "Failed to list notification channels")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(notifications.Items))
	for _, n := range notifications.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: n.Namespace,
				Name:      n.Name,
			},
		})
	}

	return requests
}

func (r *NotificationReconciler) MapTemplateToNotification(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx)

	template := obj.(*notificationv1.NotificationTemplate)

	var notifications notificationv1.NotificationList
	if err := r.Client.List(ctx, &notifications, client.MatchingFields{IndexFieldSpecTemplateNames: template.Name}); err != nil {
		log.Error(err, "Failed to list notification templates")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(notifications.Items))
	for _, n := range notifications.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: n.Namespace,
				Name:      n.Name,
			},
		})
	}

	return requests
}
