// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	"github.com/go-logr/logr"
	v1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ NotificationAdapter[v1.WebhookConfig] = &WebhookAdapter{}

type WebhookAdapter struct {
}

func (e WebhookAdapter) Send(ctx context.Context, config *v1.WebhookConfig, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Sending via webhook ", title, " ", body)

	return nil
}
