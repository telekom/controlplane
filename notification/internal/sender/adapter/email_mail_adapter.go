// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	"github.com/go-logr/logr"
	v1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ NotificationAdapter[v1.EmailConfig] = &EmailAdapter{}

type EmailAdapter struct {
}

func (e EmailAdapter) Send(ctx context.Context, config *v1.EmailConfig, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Sending via email ", title, " ", body)

	return nil
}
