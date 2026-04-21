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

// SecretRotationClientAttribute is the Keycloak client attribute key used by
// the client-attributes policy condition to identify clients that opted into
// graceful secret rotation.
const SecretRotationClientAttribute = "controlplane.secret-rotation"

func MapToClientRepresentation(client *identityv1.Client) api.ClientRepresentation {
	rep := api.ClientRepresentation{
		ClientId:               ptr.To(client.Spec.ClientId),
		Name:                   ptr.To(client.Spec.ClientId),
		Enabled:                ptr.To(true),
		FullScopeAllowed:       ptr.To(false),
		ServiceAccountsEnabled: ptr.To(true),
		StandardFlowEnabled:    ptr.To(false),
		Secret:                 ptr.To(client.Spec.ClientSecret),
		ProtocolMappers:        &[]api.ProtocolMapperRepresentation{protocolmappers.NewClientIdProtocolMapper()},
	}

	if client.SupportsSecretRotation() {
		rep.Attributes = &map[string]interface{}{
			SecretRotationClientAttribute: "true",
		}
	}

	return rep
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
		containsAllProtocolMappers(existingClient.ProtocolMappers, newClient.ProtocolMappers) &&
		compareSecretRotationAttribute(existingClient, newClient)
}

// compareSecretRotationAttribute returns true when both representations agree
// on the controlplane.secret-rotation attribute value. The attribute is
// considered "set" only when its value is the string "true".
func compareSecretRotationAttribute(a, b *api.ClientRepresentation) bool {
	return getSecretRotationAttr(a) == getSecretRotationAttr(b)
}

// getSecretRotationAttr returns "true" when the secret-rotation attribute is
// present and set to "true", otherwise "".
func getSecretRotationAttr(c *api.ClientRepresentation) string {
	if c.Attributes == nil {
		return ""
	}
	v, ok := (*c.Attributes)[SecretRotationClientAttribute]
	if !ok {
		return ""
	}
	if s, ok := v.(string); ok && s == "true" {
		return "true"
	}
	return ""
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

	// Merge the secret-rotation attribute into the existing attributes map,
	// preserving any Keycloak-managed attributes already present.
	if newClient.Attributes != nil {
		if existingClient.Attributes == nil {
			existingClient.Attributes = &map[string]interface{}{}
		}
		(*existingClient.Attributes)[SecretRotationClientAttribute] = (*newClient.Attributes)[SecretRotationClientAttribute]
	} else {
		// New client doesn't set the attribute — remove it from existing if present.
		if existingClient.Attributes != nil {
			delete(*existingClient.Attributes, SecretRotationClientAttribute)
		}
	}

	return existingClient
}
