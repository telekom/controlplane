// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	v1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ NotificationAdapter[v1.MailConfig] = &EmailAdapter{}

type EmailAdapter struct {
}

func (e EmailAdapter) Send(ctx context.Context, config v1.MailConfig, title string, body string) error {

	//TODO implement me
	panic("implement me")
}
