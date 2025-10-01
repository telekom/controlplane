// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package sender

import (
	"context"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
	"github.com/telekom/controlplane/notification/internal/sender/data"
)

type NotificationSender interface {
	ProcessNotification(ctx context.Context, data data.NotificationData) error
}

var _ NotificationSender = AdapterSender{}

type AdapterSender struct {
}

func (a AdapterSender) ProcessNotification(ctx context.Context, d data.NotificationData) error {

	switch bla := d.NotificationType(); bla {
	case data.NotificationTypeMail:
		// safe to do
		mnd := d.(data.MailNotificationData)

		// TODO should come from registry
		emailAdapter := adapter.EmailAdapter{}

		err := emailAdapter.Send(ctx, mnd.Config, mnd.NotificationDataBody.Title, mnd.NotificationDataBody.Body)
		if err != nil {
			return err
		}
	case data.NotificationTypeChat:
		// safe to do
		cnd := d.(data.ChatNotificationData)

		// TODO should come from registry
		chatAdapter := adapter.MsTeamsAdapter{}

		err := chatAdapter.Send(ctx, cnd.Config, cnd.NotificationDataBody.Title, cnd.NotificationDataBody.Body)
		if err != nil {
			return err
		}

	case data.NotificationTypeCallback:
		// safe to do
		cnd := d.(data.CallbackNotificationData)

		// TODO should come from registry
		callbackAdapter := adapter.WebhookAdapter{}

		err := callbackAdapter.Send(ctx, cnd.Config, cnd.NotificationDataBody.Title, cnd.NotificationDataBody.Body)
		if err != nil {
			return err
		}
	default:
		panic("unknown notification type")
	}

	return nil
}
