// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// NopService implements Service without performing external requests.
type NopService struct{}

func (NopService) CreateOrUpdateSFTPUser(context.Context, RoverSftpUserModel) error {
	return nil
}

func (NopService) UpdatePublicKeysForSFTPUser(context.Context, string, string, ClientPublicKeyMap) error {
	return nil
}

func (NopService) DeleteSFTPUser(context.Context, string) error {
	return nil
}

func NewNopFactory() Factory {
	return FactoryFunc(func(context.Context, client.ObjectKey) (Service, error) {
		return NopService{}, nil
	})
}

type NopClientManager struct{}

func (NopClientManager) ServiceFor(context.Context, client.ObjectKey) (Service, error) {
	return NopService{}, nil
}

func (NopClientManager) IsServiceCached(client.ObjectKey) bool {
	return true
}

func (NopClientManager) CreateOrUpdate(context.Context, *sftpv1.ZoneServiceConfig) error {
	return nil
}

func (NopClientManager) Delete(*sftpv1.ZoneServiceConfig) {}

func NewNopClientManager() ClientManager {
	return NopClientManager{}
}
