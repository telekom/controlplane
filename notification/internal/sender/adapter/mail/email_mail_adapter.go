// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mail

import (
	"context"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/notification/internal/config"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
)

var _ adapter.NotificationAdapter[adapter.MailChannelConfiguration] = &EmailAdapter{}

type EmailAdapter struct {
	AdapterConfig *config.EmailAdapterConfig
}

func (e EmailAdapter) Send(ctx context.Context, channelConfig adapter.MailChannelConfiguration, title string, body string) error {
	smtpSender := NewSMTPSender(e.AdapterConfig)

	var from string
	if channelConfig.GetFrom() != nil {
		from = *channelConfig.GetFrom()
	} else {
		from = e.AdapterConfig.SMTPSender.DefaultFrom
	}

	err := smtpSender.Send(ctx, from, e.AdapterConfig.SMTPSender.DefaultName, channelConfig.GetRecipients(), title, body)
	if err != nil {
		return errors.Wrap(err, "Failed to send email via SMTPSender")
	}

	return nil
}
