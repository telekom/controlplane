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
	MailAdapter     adapter.NotificationAdapter[adapter.MailChannelConfiguration]
	ChatAdapter     adapter.NotificationAdapter[adapter.ChatChannelConfiguration]
	CallbackAdapter adapter.NotificationAdapter[adapter.CallbackChannelConfiguration]
}

func (a AdapterSender) ProcessNotification(ctx context.Context, channel *notificationv1.NotificationChannel, subject string, body string) error {

	var err error
	switch nType := channel.NotificationType(); nType {
	case notificationv1.NotificationTypeMail:
		err = a.MailAdapter.Send(ctx, channel.Spec.Email, subject, body)
	case notificationv1.NotificationTypeChat:
		err = a.ChatAdapter.Send(ctx, channel.Spec.MsTeams, subject, body)
	case notificationv1.NotificationTypeCallback:
		err = a.CallbackAdapter.Send(ctx, channel.Spec.Webhook, subject, body)
	default:
		err = errors.New("unknown notification type")
	}

	return err
}
