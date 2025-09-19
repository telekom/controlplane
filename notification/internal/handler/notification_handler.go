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

var _ handler.Handler[*notificationv1.Notification] = &NotificationHandler{}

type NotificationHandler struct {
}

func (n *NotificationHandler) CreateOrUpdate(ctx context.Context, notification *notificationv1.Notification) error {
	notification.SetCondition(condition.NewReadyCondition("Provisioned", "Notification is provisioned"))
	notification.SetCondition(condition.NewDoneProcessingCondition("Notification is done processing"))
	return nil
}

func (n *NotificationHandler) Delete(ctx context.Context, notification *notificationv1.Notification) error {
	return nil
}
