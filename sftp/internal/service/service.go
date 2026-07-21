// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

// Service provides the operations exposed by the SFTP Tardis API.
type Service interface {
	CreateOrUpdateSFTPUser(ctx context.Context, user RoverSftpUserModel) error
	UpdatePublicKeysForSFTPUser(ctx context.Context, sftpUserName, clientID string, keys ClientPublicKeyMap) error
	DeleteSFTPUser(ctx context.Context, sftpUserName string) error
}

// Factory creates a Service for an SFTPServiceConfig.
type Factory interface {
	ServiceFor(ctx context.Context, sftpServiceConfig client.ObjectKey) (Service, error)
}

// ClientManager manages SFTP API clients configured by SFTPServiceConfig resources.
// It provides reusability of clients and ensures that the clients are initialized only once per SFTPServiceConfig.
type ClientManager interface {
	Factory
	// ExistClient returns true if a client for the given SFTPServiceConfig is already initialized and available to use.
	ExistClient(sftpServiceConfig client.ObjectKey) bool
	// CreateOrUpdate creates or updates the SFTP API client for the given SFTPServiceConfig in client manager
	// It initialize oauth2 client credentials and creates a new SFTP API client if it does not exist yet.
	CreateOrUpdate(ctx context.Context, sftpServiceConfig *sftpv1.SFTPServiceConfig) error
	// Delete removes the client for the given SFTPServiceConfig from the client manager.
	Delete(sftpServiceConfig *sftpv1.SFTPServiceConfig)
}

// FactoryFunc adapts a function to the Factory interface.
type FactoryFunc func(ctx context.Context, sftpServiceConfig client.ObjectKey) (Service, error)

func (f FactoryFunc) ServiceFor(ctx context.Context, sftpServiceConfig client.ObjectKey) (Service, error) {
	return f(ctx, sftpServiceConfig)
}
