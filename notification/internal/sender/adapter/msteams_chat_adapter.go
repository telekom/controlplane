// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	"github.com/go-logr/logr"
	v1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ NotificationAdapter[v1.MsTeamsConfig] = &MsTeamsAdapter{}

type MsTeamsAdapter struct {
}

func (e MsTeamsAdapter) Send(ctx context.Context, config *v1.MsTeamsConfig, title string, body string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Sending via msteams ", title, " ", body)

	return nil
}
