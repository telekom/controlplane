// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

const (
	IndexFieldSpecChannelRefs   = "spec.channelRefs"
	IndexFieldSpecTemplateNames = "spec.templateNames"
)

func RegisterIndecesOrDie(ctx context.Context, mgr ctrl.Manager) {
	// Index Notifications by each channel reference
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&notificationv1.Notification{},
		IndexFieldSpecChannelRefs,
		func(obj client.Object) []string {
			n, ok := obj.(*notificationv1.Notification)
			if !ok {
				return nil
			}
			keys := make([]string, 0, len(n.Spec.Channels))
			for _, ch := range n.Spec.Channels {
				keys = append(keys, fmt.Sprintf("%s/%s", ch.Namespace, ch.Name))
			}
			return keys
		},
	); err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Notification", "FieldIndex", IndexFieldSpecChannelRefs)
		os.Exit(1)
	}

	// Index Notifications by each template
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&notificationv1.Notification{},
		IndexFieldSpecTemplateNames,
		func(obj client.Object) []string {
			n, ok := obj.(*notificationv1.Notification)
			if !ok {
				return nil
			}
			keys := make([]string, 0, len(n.Spec.Channels))
			for _, ch := range n.Spec.Channels {
				parts := strings.Split(ch.Name, "--")
				channelType := parts[len(parts)-1]
				keys = append(keys, fmt.Sprintf("%s--%s", n.Spec.Purpose, channelType))
			}
			return keys
		},
	); err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for Notification", "FieldIndex", IndexFieldSpecTemplateNames)
		os.Exit(1)
	}
}

func (r *NotificationReconciler) MapChannelToNotification(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	ch, ok := obj.(*notificationv1.NotificationChannel)
	if !ok {
		return nil
	}
	channelKey := fmt.Sprintf("%s/%s", ch.Namespace, ch.Name)

	var notifications notificationv1.NotificationList
	if err := r.List(ctx, &notifications, client.MatchingFields{IndexFieldSpecChannelRefs: channelKey}); err != nil {
		logger.Error(err, "Failed to list notification channels")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(notifications.Items))
	for i := range notifications.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: notifications.Items[i].Namespace,
				Name:      notifications.Items[i].Name,
			},
		})
	}

	return requests
}

func (r *NotificationReconciler) MapTemplateToNotification(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	template, ok := obj.(*notificationv1.NotificationTemplate)
	if !ok {
		return nil
	}

	var notifications notificationv1.NotificationList
	if err := r.List(ctx, &notifications, client.MatchingFields{IndexFieldSpecTemplateNames: template.Name}); err != nil {
		logger.Error(err, "Failed to list notification templates")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(notifications.Items))
	for i := range notifications.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: notifications.Items[i].Namespace,
				Name:      notifications.Items[i].Name,
			},
		})
	}

	return requests
}
