// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sftpv1 "github.com/telekom/controlplane/sftp/api/v1"
)

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../api/service/api.yaml

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
type ClientManager interface {
	Factory
	IsServiceCached(sftpServiceConfig client.ObjectKey) bool
	CreateOrUpdate(ctx context.Context, sftpServiceConfig *sftpv1.SFTPServiceConfig) error
	Delete(sftpServiceConfig *sftpv1.SFTPServiceConfig)
}

// FactoryFunc adapts a function to the Factory interface.
type FactoryFunc func(ctx context.Context, sftpServiceConfig client.ObjectKey) (Service, error)

func (f FactoryFunc) ServiceFor(ctx context.Context, sftpServiceConfig client.ObjectKey) (Service, error) {
	return f(ctx, sftpServiceConfig)
}
