// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/internal/config"
)

const (
	IndexFieldSpecChannelRefs = "spec.channelRefs"
)

var _ handler.Handler[*notificationv1.NotificationChannel] = &NotificationChannelHandler{}

type NotificationChannelHandler struct {
	HousekeepingConfig *config.NotificationHousekeepingConfig
}

func (n *NotificationChannelHandler) CreateOrUpdate(ctx context.Context, channel *notificationv1.NotificationChannel) error {
	doNotificationsHousekeeping(ctx, channel, n.HousekeepingConfig)

	channel.SetCondition(condition.NewReadyCondition("Provisioned", "Notification channel is provisioned"))
	channel.SetCondition(condition.NewDoneProcessingCondition("Notification channel is done processing"))
	return nil
}

func (n *NotificationChannelHandler) Delete(ctx context.Context, channel *notificationv1.NotificationChannel) error {
	return nil
}

func doNotificationsHousekeeping(ctx context.Context, channel *notificationv1.NotificationChannel, housekeepingConfig *config.NotificationHousekeepingConfig) {
	logger := log.FromContext(ctx)

	var notifications notificationv1.NotificationList
	channelKey := fmt.Sprintf("%s/%s", channel.Namespace, channel.Name)

	scopedClient := cclient.ClientFromContextOrDie(ctx)
	if err := scopedClient.List(ctx, &notifications, client.MatchingFields{IndexFieldSpecChannelRefs: channelKey}); err != nil {
		logger.Error(err, fmt.Sprintf("Failed to list notifications for channel %q ", channelKey))
		return
	}

	for i := range notifications.Items {
		if eligibleForHousekeeping(ctx, &notifications.Items[i], housekeepingConfig.TTLMonthsAfterFinished) {
			err := scopedClient.Delete(ctx, &notifications.Items[i])
			if err != nil {
				logger.V(0).Error(err, "Failed to delete expired notification", "name", notifications.Items[i].Name)
			} else {
				logger.V(0).Info("Deleted expired notification", "name", notifications.Items[i].Name)
			}
		}
	}
}

func eligibleForHousekeeping(ctx context.Context, notification *notificationv1.Notification, ttlMonthsAfterFinished int32) bool {
	logger := log.FromContext(ctx)

	// check if it's ready
	ready := meta.IsStatusConditionTrue(notification.GetConditions(), condition.ConditionTypeReady)
	if !ready {
		return false
	}

	// check if it's complete (all channels successfully sent)
	if !isNotificationComplete(notification) {
		return false
	}

	// get timestamp when the notification became ready
	var readyTimestamp time.Time
	for _, c := range notification.GetConditions() {
		if c.Type == condition.ConditionTypeReady {
			readyTimestamp = c.LastTransitionTime.Time
			break
		}
	}

	ttl := time.Duration(ttlMonthsAfterFinished) * time.Hour * 24 * 7 * 4
	expiry := readyTimestamp.Add(ttl)
	if time.Now().After(expiry) {
		logger.V(1).Info("Notification is expired and eligible for housekeeping")
		return true
	} else {
		return false
	}
}
