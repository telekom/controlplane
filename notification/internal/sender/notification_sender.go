// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package sender

import (
	"context"
	"github.com/pkg/errors"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
)

type NotificationSender interface {
	ProcessNotification(ctx context.Context, channel *notificationv1.NotificationChannel, subject string, body string) error
}

var _ NotificationSender = AdapterSender{}

type AdapterSender struct {
}

func (a AdapterSender) ProcessNotification(ctx context.Context, channel *notificationv1.NotificationChannel, subject string, body string) error {

	var err error
	switch nType := channel.NotificationType(); nType {
	case notificationv1.NotificationTypeMail:
		emailAdapter := adapter.EmailAdapter{}

		err = emailAdapter.Send(ctx, channel.Spec.Email, subject, body)
	case notificationv1.NotificationTypeChat:
		chatAdapter := adapter.MsTeamsAdapter{}

		err = chatAdapter.Send(ctx, channel.Spec.MsTeams, subject, body)
	case notificationv1.NotificationTypeCallback:
		webhookAdapter := adapter.WebhookAdapter{}

		err = webhookAdapter.Send(ctx, channel.Spec.Webhook, subject, body)
	default:
		err = errors.New("unknown notification type")
	}

	if err != nil {
		return err
	}

	return nil
}
