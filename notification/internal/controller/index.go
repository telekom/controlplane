// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"k8s.io/apimachinery/pkg/types"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
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
		ctrl.Log.Error(err, "unable to create fieldIndex for Notification", "FieldIndex", IndexFieldSpecChannelRefs)
		os.Exit(1)
	}

	// Index Notifications by each template
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
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
		ctrl.Log.Error(err, "unable to create fieldIndex for Notification", "FieldIndex", IndexFieldSpecTemplateNames)
		os.Exit(1)
	}
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
