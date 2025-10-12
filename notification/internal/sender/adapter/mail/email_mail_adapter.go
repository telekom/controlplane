// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mail

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/notification/internal/config"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
)

var _ adapter.NotificationAdapter[adapter.MailConfiguration] = &EmailAdapter{}

type EmailAdapter struct {
	Config *config.EmailAdapterConfig
}

func (e EmailAdapter) Send(ctx context.Context, config adapter.MailConfiguration, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)

	_ = NewSMTPSender(e.Config)

	// either use the default smtp config or it can be overridden in the channel
	log.Info("Sending via email ", "title", title, "body", body)

	return nil
}
