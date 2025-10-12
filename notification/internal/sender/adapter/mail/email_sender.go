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
	"net"
	"strings"
	"time"
)

type EmailSender interface {
	Send(ctx context.Context, from string, senderName string, bcc []string, subject, body string) error
}

type SMTPEmailSender struct {
	config *config.EmailAdapterConfig
}

type EmailResult struct {
	Address string
	Success bool
	Error   error
}

func NewSMTPSender(config *config.EmailAdapterConfig) *SMTPEmailSender {
	return &SMTPEmailSender{config: config}
}

func (s *SMTPEmailSender) Send(ctx context.Context, from string, senderName string, bcc []string, subject, body string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Timeout für die gesamte Operation setzen
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
		return fmt.Errorf("failed to send email: %v", err)
	}

	log.Info("Email sent successfully to %v", bcc)
	return nil
}

func (s *SMTPEmailSender) SendEmailsInBatches(ctx context.Context, sender EmailSender, from string, senderName string, bccRecipients []string, subject, body string) ([]EmailResult, error) {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Started function SendEmailsInBatches")
	var results []EmailResult

	batchSize := s.config.SMTPSender.BatchSize
	for i := 0; i < len(bccRecipients); i += batchSize {
		select {
		case <-ctx.Done():
			log.Info("Context canceled in SendEmailsInBatches")
			return results, ctx.Err()
		default:
			end := i + batchSize
			if end > len(bccRecipients) {
				end = len(bccRecipients)
			}
			batch := bccRecipients[i:end]

			log.V(1).Info("Sending batch %d to %d", i, end)

			err := s.retryWithBackoff(ctx, func() error {
				return sender.Send(ctx, from, senderName, batch, subject, body)
			})

			if err != nil {
				log.Error(err, "Error sending batch")
				for _, addr := range batch {
					results = append(results, EmailResult{
						Address: addr,
						Success: false,
						Error:   err,
					})
				}
			} else {
				log.V(1).Info("Batch sent successfully")
				for _, addr := range batch {
					results = append(results, EmailResult{
						Address: addr,
						Success: true,
						Error:   nil,
					})
				}
			}

			log.V(1).Info("Sleeping for rate limiting")
			time.Sleep(1 * s.config.SMTPSender.BatchLoopDelay)
		}
	}

	log.V(1).Info("Finished SendEmailsInBatches")
	return results, nil
}

func (s *SMTPEmailSender) retryWithBackoff(ctx context.Context, fn func() error) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Started function retryWithBackoff")
	var err error
	backoff := s.config.SMTPSender.InitialBackoff
	maxRetries := s.config.SMTPSender.MaxRetries
	maxBackoff := s.config.SMTPSender.MaxBackoff

	for retry := 0; retry < maxRetries; retry++ {
		select {
		case <-ctx.Done():
			log.Error(ctx.Err(), "Context canceled in retryWithBackoff")
			return ctx.Err()
		default:
			err = fn()
			if err == nil {
				return nil
			}

			log.V(1).Info("Attempt %d failed: %v", retry+1, err)

			if !isTemporaryError(err) {
				log.V(1).Info("Non-temporary error encountered")
				return err
			}

			log.V(1).Info("Retrying in %v", backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return errors.Wrap(err, "Max retries reached")
}

func isTemporaryError(err error) bool {

	// Check for specific net.OpError conditions that are temporary
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Temporary() {
		if opErr.Timeout() {
			return true
		}
		// Check if the underlying error is a net.DNSError and is temporary
		var dnsErr *net.DNSError
		if errors.As(opErr.Err, &dnsErr) && dnsErr.IsTemporary {
			return true
		}
	}

	// Check if the error is a net.DNSError and is temporary
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsTemporary {
		return true
	}

	temporaryErrors := []string{
		"connection reset by peer",
		"i/o timeout",
		"EOF",
		"network is unreachable",
		"no route to host",
		"connection refused",
		"connection timed out",
		"temporary failure in name resolution",
		"421", // the SMTP service is not available and the transmission channel is closing.
		"450", // the requested mail action was not taken because the mailbox is unavailable.
		"451", // the requested action was aborted due to a local error in processing.
		"452", // the requested action was not taken due to insufficient system storage.
	}

	for _, tempErr := range temporaryErrors {
		if strings.Contains(err.Error(), tempErr) {
			return true
		}
	}

	return false
}
