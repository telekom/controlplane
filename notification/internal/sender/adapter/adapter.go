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

type MailConfiguration interface {
	IsNotificationConfig()
	GetRecipients() []string
	GetCCRecipients() []string
	GetSMTPHost() string
	GetSMTPPort() int
	GetFrom() string
}

type ChatConfiguration interface {
	IsNotificationConfig()
	GetWebhookURL() string
}

type CallbackConfiguration interface {
	IsNotificationConfig()
	GetURL() string
	GetMethod() string
	GetHeaders() map[string]string
}
