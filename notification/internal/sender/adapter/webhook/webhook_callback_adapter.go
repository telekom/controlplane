// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/notification/internal/sender/adapter"
)

var _ adapter.NotificationAdapter[adapter.CallbackChannelConfiguration] = &WebhookAdapter{}

type WebhookAdapter struct {
}

func (e WebhookAdapter) Send(ctx context.Context, config adapter.CallbackChannelConfiguration, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Sending via webhook ", title, " ", body)

	return nil
}
