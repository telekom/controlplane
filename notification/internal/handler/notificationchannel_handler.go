// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/handler"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ handler.Handler[*notificationv1.NotificationChannel] = &NotificationChannelHandler{}

type NotificationChannelHandler struct {
}

func (n *NotificationChannelHandler) CreateOrUpdate(ctx context.Context, channel *notificationv1.NotificationChannel) error {
	channel.SetCondition(condition.NewReadyCondition("Provisioned", "Notification channel is provisioned"))
	channel.SetCondition(condition.NewDoneProcessingCondition("Notification channel is done processing"))
	return nil
}

func (n *NotificationChannelHandler) Delete(ctx context.Context, channel *notificationv1.NotificationChannel) error {
	return nil
}
