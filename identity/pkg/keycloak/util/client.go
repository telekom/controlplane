// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"github.com/pkg/errors"
	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak/protocolmappers"
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
		ProtocolMappers:        &[]api.ProtocolMapperRepresentation{protocolmappers.NewClientIdProtocolMapper()},
	}
}

// HasSecretChanged returns true when the new client representation carries a
// different secret than the existing one. Both pointers must be non-nil.
func HasSecretChanged(existingClient, newClient *api.ClientRepresentation) bool {
	if existingClient.Secret == nil || newClient.Secret == nil {
		return existingClient.Secret != newClient.Secret
	}
	return *existingClient.Secret != *newClient.Secret
}

func CompareClientRepresentation(existingClient, newClient *api.ClientRepresentation) bool {
	if existingClient == nil || newClient == nil {
		return existingClient == newClient
	}
	return ptrEqual(existingClient.ClientId, newClient.ClientId) &&
		ptrEqual(existingClient.Name, newClient.Name) &&
		ptrEqual(existingClient.Enabled, newClient.Enabled) &&
		ptrEqual(existingClient.FullScopeAllowed, newClient.FullScopeAllowed) &&
		ptrEqual(existingClient.ServiceAccountsEnabled, newClient.ServiceAccountsEnabled) &&
		ptrEqual(existingClient.StandardFlowEnabled, newClient.StandardFlowEnabled) &&
		ptrEqual(existingClient.Secret, newClient.Secret) &&
		containsAllProtocolMappers(existingClient.ProtocolMappers, newClient.ProtocolMappers)
}

// ptrEqual returns true if both pointers are nil, or both are non-nil and
// point to equal values.
func ptrEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
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
