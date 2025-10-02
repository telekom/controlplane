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

var _ handler.Handler[*notificationv1.NotificationTemplate] = &NotificationTemplateHandler{}

type NotificationTemplateHandler struct {
}

func (n *NotificationTemplateHandler) CreateOrUpdate(ctx context.Context, template *notificationv1.NotificationTemplate) error {

	template.SetCondition(condition.NewReadyCondition("Provisioned", "Notification template is provisioned"))
	template.SetCondition(condition.NewDoneProcessingCondition("Notification template is done processing"))
	return nil
}

func (n *NotificationTemplateHandler) Delete(ctx context.Context, template *notificationv1.NotificationTemplate) error {
	return nil
}
