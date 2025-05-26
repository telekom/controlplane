// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
)

func GetClient(getRealmClients api.GetRealmClientsResponse) (*api.ClientRepresentation, error) {
	foundClients := getRealmClients.JSON2XX
	switch len(*foundClients) {
	case 0:
		return nil, nil
	case 1:
		existingClient := (*foundClients)[0]
		return &existingClient, nil
	default:
		return nil, errors.Errorf("unexpected number of clients: %d", len(*foundClients))
	}
}

func MapToClientRepresentation(client *identityv1.Client) api.ClientRepresentation {
	return api.ClientRepresentation{
		ClientId:               ptr.To(client.Spec.ClientId),
		Name:                   ptr.To(client.Spec.ClientId),
		Enabled:                ptr.To(true),
		FullScopeAllowed:       ptr.To(false),
		ServiceAccountsEnabled: ptr.To(true),
		StandardFlowEnabled:    ptr.To(false),
		Secret:                 ptr.To(client.Spec.ClientSecret),
		ProtocolMappers:        &[]api.ProtocolMapperRepresentation{MapToProtocolMapperRepresentation()},
	}
}

func CompareClientRepresentation(existingClient, newClient *api.ClientRepresentation) bool {
	return *existingClient.ClientId == *newClient.ClientId &&
		*existingClient.Name == *newClient.Name &&
		*existingClient.Enabled == *newClient.Enabled &&
		*existingClient.FullScopeAllowed == *newClient.FullScopeAllowed &&
		*existingClient.ServiceAccountsEnabled == *newClient.ServiceAccountsEnabled &&
		*existingClient.StandardFlowEnabled == *newClient.StandardFlowEnabled &&
		*existingClient.Secret == *newClient.Secret &&
		containsAllProtocolMappers(existingClient.ProtocolMappers, newClient.ProtocolMappers)
}

func MergeClientRepresentation(existingClient, newClient *api.ClientRepresentation) *api.ClientRepresentation {
	existingClient.ClientId = newClient.ClientId
	existingClient.Name = newClient.Name
	existingClient.Enabled = newClient.Enabled
	existingClient.FullScopeAllowed = newClient.FullScopeAllowed
	existingClient.ServiceAccountsEnabled = newClient.ServiceAccountsEnabled
	existingClient.StandardFlowEnabled = newClient.StandardFlowEnabled
	existingClient.Secret = newClient.Secret
	existingClient.ProtocolMappers = MergeProtocolMappers(existingClient.ProtocolMappers, newClient.ProtocolMappers)

	return existingClient
}
