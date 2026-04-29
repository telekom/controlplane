// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mail

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/notification/internal/config"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
)

var _ adapter.NotificationAdapter[adapter.MailChannelConfiguration] = &EmailAdapter{}

type EmailAdapter struct {
	AdapterConfig *config.EmailAdapterConfig
}

// Send delivers an email notification through the configured SMTP sender.
func (e EmailAdapter) Send(ctx context.Context, channelConfig adapter.MailChannelConfiguration, title, body string, attachments []adapter.Attachment) error {
	log := logr.FromContextOrDiscard(ctx)
	smtpSender := NewSMTPSender(e.AdapterConfig)

	var from string
	if channelConfig.GetFrom() != nil {
		from = *channelConfig.GetFrom()
	} else {
		from = e.AdapterConfig.SMTPSender.DefaultFrom
	}

	// handle dry run
	if e.AdapterConfig.SMTPSender.DryRun {
		log.V(1).Info("Dry run - would send email", "from", from, "name", e.AdapterConfig.SMTPSender.DefaultName, "recipients", channelConfig.GetRecipients(), "title", title, "body", body, "attachmentCount", len(attachments))
	} else {
		err := smtpSender.Send(ctx, from, e.AdapterConfig.SMTPSender.DefaultName, channelConfig.GetRecipients(), title, body, attachments)
		if err != nil {
			return errors.Wrap(err, "Failed to send email via SMTPSender")
		}
	}

	return nil
}
