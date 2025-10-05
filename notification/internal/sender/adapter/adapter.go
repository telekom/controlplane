// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
)

type NotificationAdapter[C any] interface {
	Send(ctx context.Context, config C, title string, body string) error
}

type MailConfiguration interface {
	GetRecipients() []string
	GetCCRecipients() []string
	GetSMTPHost() string
	GetSMTPPort() int
	GetFrom() string
}

type ChatConfiguration interface {
	GetWebhookURL() string
}

type CallbackConfiguration interface {
	GetURL() string
	GetMethod() string
	GetHeaders() map[string]string
}
