// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package syncer

import (
	"context"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
)

const NOT_UPDATED = false

type SyncerClientMock struct {
}

func NewSyncerFactoryMock() SyncerClientFactory[*apiv1.RemoteApiSubscription] {
	return &syncerFactory[*apiv1.RemoteApiSubscription]{
		New: func(cfg SyncerClientConfig) SyncerClient[*apiv1.RemoteApiSubscription] {
			return &SyncerClientMock{}
		},
	}
}

func (c *SyncerClientMock) Send(ctx context.Context, resource *apiv1.RemoteApiSubscription) (bool, *apiv1.RemoteApiSubscription, error) {
	return NOT_UPDATED, resource, nil
}

func (c *SyncerClientMock) SendStatus(ctx context.Context, resource *apiv1.RemoteApiSubscription) (bool, *apiv1.RemoteApiSubscription, error) {
	return NOT_UPDATED, resource, nil
}

func (c *SyncerClientMock) Delete(ctx context.Context, resource *apiv1.RemoteApiSubscription) error {
	return nil
}
