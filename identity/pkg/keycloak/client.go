// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

type RealmClient interface {
	// Realm related operations

	GetRealm(ctx context.Context, realm string) (*api.GetRealmResponse, error)
	PutRealm(ctx context.Context, realmName string, realm *identityv1.Realm) (*api.PutRealmResponse, error)
	PostRealm(ctx context.Context, realm *identityv1.Realm) (*api.PostResponse, error)
	CreateOrUpdateRealm(ctx context.Context, realm *identityv1.Realm) error
	DeleteRealm(ctx context.Context, realm string) error

	// RealmClient related operations

	GetRealmClients(ctx context.Context, realm string,
		client *identityv1.Client) (*api.GetRealmClientsResponse, error)
	PutRealmClient(ctx context.Context, realmName, id string,
		client *identityv1.Client) (*api.PutRealmClientsIdResponse, error)
	PostRealmClient(ctx context.Context, realmName string,
		client *identityv1.Client) (*api.PostRealmClientsResponse, error)
	CreateOrUpdateRealmClient(ctx context.Context, realm *identityv1.Realm, client *identityv1.Client) error
	DeleteRealmClient(ctx context.Context, realmName, id string) error
}
