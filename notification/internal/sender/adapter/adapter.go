// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
)

type NotificationConfig interface {
	IsNotificationConfig()
}

type NotificationAdapter[C NotificationConfig] interface {
	Send(ctx context.Context, config C, title string, body string) error
}

type MailChannelConfiguration interface {
	IsNotificationConfig()
	GetRecipients() []string
	GetFrom() *string
}

type ChatChannelConfiguration interface {
	IsNotificationConfig()
	GetWebhookURL() string
}

type CallbackChannelConfiguration interface {
	IsNotificationConfig()
	GetURL() string
	GetMethod() string
	GetHeaders() map[string]string
}
