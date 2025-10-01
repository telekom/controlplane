// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package adapter

import (
	"context"
	v1 "github.com/telekom/controlplane/notification/api/v1"
)

var _ NotificationAdapter[v1.ChatConfig] = &MsTeamsAdapter{}

type MsTeamsAdapter struct {
}

func (e MsTeamsAdapter) Send(ctx context.Context, config v1.ChatConfig, title string, body string) error {

	//TODO implement me
	panic("implement me")
}
