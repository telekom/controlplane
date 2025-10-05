// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	"github.com/go-logr/logr"
)

var _ NotificationAdapter[MailConfiguration] = &EmailAdapter{}

type EmailAdapter struct {
	SMTPHost string
	SMTPPort int
}

func (e EmailAdapter) Send(ctx context.Context, config MailConfiguration, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Sending via email ", title, " ", body)

	return nil
}
