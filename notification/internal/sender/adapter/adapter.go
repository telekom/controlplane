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

// Attachment is a rendered file ready to attach to a notification.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

type NotificationAdapter[C NotificationConfig] interface {
	Send(ctx context.Context, config C, title string, body string, attachments []Attachment) error
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
