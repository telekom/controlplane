// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"gopkg.in/gomail.v2"

	"github.com/telekom/controlplane/notification/internal/config"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
)

var _ EmailSender = SMTPEmailSender{}

type EmailSender interface {
	Send(ctx context.Context, from, senderName string, bcc []string, subject, body string, attachments []adapter.Attachment) error
}

type SMTPEmailSender struct {
	config *config.EmailAdapterConfig
}

var NewSMTPSender = func(config *config.EmailAdapterConfig) EmailSender {
	return &SMTPEmailSender{config: config}
}

// Send delivers an email via SMTP with optional file attachments.
func (s SMTPEmailSender) Send(ctx context.Context, from, senderName string, bcc []string, subject, body string, attachments []adapter.Attachment) error {
	log := logr.FromContextOrDiscard(ctx)

	_, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	d := gomail.NewDialer(s.config.SMTPConnection.Host, s.config.SMTPConnection.Port, s.config.SMTPConnection.User, s.config.SMTPConnection.Password)

	// we are aware that the InsecureSkipVerify is set to true. communication is within cluster and this is currently acceptable
	d.TLSConfig = &tls.Config{ServerName: s.config.SMTPConnection.Host, InsecureSkipVerify: true} //nolint:gosec // G402: intra-cluster communication, acceptable risk

	m := gomail.NewMessage()
	m.SetHeader("From", fmt.Sprintf("%s <%s>", senderName, from))
	m.SetHeader("Bcc", bcc...)
	m.SetHeader("Subject", subject)
	m.SetHeader("X-Mailer", "Hyperion Notifier")
	m.SetBody("text/html", body)

	for _, att := range attachments {
		m.Attach(att.Filename,
			gomail.SetCopyFunc(func(w io.Writer) error {
				_, err := io.Copy(w, bytes.NewReader(att.Content))
				return err
			}),
			gomail.SetHeader(map[string][]string{
				"Content-Type": {att.ContentType},
			}),
		)
	}

	if err := d.DialAndSend(m); err != nil {
		return errors.Wrap(err, "Failed to send email")
	}

	log.Info("Email sent successfully", "bcc", bcc)
	return nil
}
