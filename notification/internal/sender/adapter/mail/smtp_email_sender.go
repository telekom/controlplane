// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/notification/internal/config"
	"gopkg.in/gomail.v2"
	"time"
)

var _ EmailSender = SMTPEmailSender{}

type EmailSender interface {
	Send(ctx context.Context, from string, senderName string, bcc []string, subject, body string) error
}

type SMTPEmailSender struct {
	config *config.EmailAdapterConfig
}

var NewSMTPSender = func(config *config.EmailAdapterConfig) EmailSender {
	return &SMTPEmailSender{config: config}
}

func (s SMTPEmailSender) Send(ctx context.Context, from string, senderName string, bcc []string, subject, body string) error {
	log := logr.FromContextOrDiscard(ctx)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	d := gomail.NewDialer(s.config.SMTPConnection.Host, s.config.SMTPConnection.Port, s.config.SMTPConnection.User, s.config.SMTPConnection.Password)
	d.TLSConfig = &tls.Config{ServerName: s.config.SMTPConnection.Host, InsecureSkipVerify: true}

	m := gomail.NewMessage()
	m.SetHeader("From", fmt.Sprintf("%s <%s>", senderName, from))
	m.SetHeader("Bcc", bcc...)
	m.SetHeader("Subject", subject)
	m.SetHeader("MIME-Version", "1.0")
	m.SetHeader("Content-Type", "text/html; charset=UTF-8")
	m.SetHeader("X-Mailer", "Hyperion Notifier")
	m.SetBody("text/html", body)

	if err := d.DialAndSend(m); err != nil {
		return errors.Wrap(err, "Failed to send email")
	}

	log.Info("Email sent successfully", "bcc", bcc)
	return nil
}
