// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"slices"

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
const (
	SecretRotationClientAttribute = "controlplane.secret-rotation"
	stringTrue                    = "true"
)

// Client attributes written by Keycloak's secret-rotation executor.
const (
	SecretCreationTimeAttribute          = "client.secret.creation.time"
	SecretExpirationTimeAttribute        = "client.secret.expiration.time"
	SecretRemainingExpirationAttribute   = "client.secret.remaining.expiration.time"
	RotatedSecretAttribute               = "client.secret.rotated"
	RotatedSecretCreationTimeAttribute   = "client.secret.rotated.creation.time"
	RotatedSecretExpirationTimeAttribute = "client.secret.rotated.expiration.time"
)

// secretRotationStateAttributes must be removed when a client opts out of
// rotation. Keycloak enforces them on every client authentication even when no
// rotation policy applies anymore: the current secret is rejected once
// client.secret.expiration.time has passed, and the rotated secret stays valid.
var secretRotationStateAttributes = []string{
	SecretExpirationTimeAttribute,
	SecretRemainingExpirationAttribute,
	RotatedSecretAttribute,
	RotatedSecretCreationTimeAttribute,
	RotatedSecretExpirationTimeAttribute,
}

func MapToClientRepresentation(client *identityv1.Client) api.ClientRepresentation {
	webOrigins := slices.Clone(client.Status.AllowedOrigins)
	rep := api.ClientRepresentation{
		ClientId:               ptr.To(client.Spec.ClientId),
		Name:                   ptr.To(client.Spec.ClientId),
		Enabled:                ptr.To(true),
		FullScopeAllowed:       ptr.To(false),
		ServiceAccountsEnabled: ptr.To(true),
		StandardFlowEnabled:    ptr.To(false),
		Secret:                 ptr.To(client.Spec.ClientSecret),
		WebOrigins:             mapWebOrigins(webOrigins),
		ProtocolMappers:        &[]api.ProtocolMapperRepresentation{protocolmappers.NewClientIdProtocolMapper()},
	}

	if client.SupportsSecretRotation() {
		rep.Attributes = &map[string]interface{}{
			SecretRotationClientAttribute: stringTrue,
		}
	}

	return rep
}

func mapWebOrigins(origins []string) *[]string {
	if len(origins) == 0 {
		// we have to return pointer to an empty slice
		// nil won't delete already configured web origins in Keycloak.
		return &[]string{}
	}
	return &origins
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
		compareWebOrigins(existingClient.WebOrigins, newClient.WebOrigins) &&
		containsAllProtocolMappers(existingClient.ProtocolMappers, newClient.ProtocolMappers) &&
		compareSecretRotationAttribute(existingClient, newClient)
}

func compareWebOrigins(left, right *[]string) bool {
	var leftOrigins, rightOrigins []string
	if left != nil {
		leftOrigins = slices.Clone(*left)
		slices.Sort(leftOrigins)
	}
	if right != nil {
		rightOrigins = slices.Clone(*right)
		slices.Sort(rightOrigins)
	}

	return slices.Equal(leftOrigins, rightOrigins)
}

// compareSecretRotationAttribute returns true when both representations agree
// on the controlplane.secret-rotation attribute value. The attribute is
// considered "set" only when its value is the string "true". When the desired
// client opted out of rotation, any leftover rotation state on the existing
// client counts as drift.
func compareSecretRotationAttribute(existing, desired *api.ClientRepresentation) bool {
	if getSecretRotationAttr(existing) != getSecretRotationAttr(desired) {
		return false
	}
	return getSecretRotationAttr(desired) == stringTrue || !hasSecretRotationState(existing)
}

func hasSecretRotationState(c *api.ClientRepresentation) bool {
	if c.Attributes == nil {
		return false
	}
	for _, key := range secretRotationStateAttributes {
		if isAttributeSet(*c.Attributes, key) {
			return true
		}
	}
	return false
}

// isAttributeSet mirrors Keycloak, which treats empty values as absent.
func isAttributeSet(attrs map[string]interface{}, key string) bool {
	v, ok := attrs[key]
	if !ok || v == nil {
		return false
	}
	s, isString := v.(string)
	return !isString || s != ""
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
	if s, ok := v.(string); ok && s == stringTrue {
		return stringTrue
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
	existingClient.WebOrigins = newClient.WebOrigins
	existingClient.ProtocolMappers = MergeProtocolMappers(existingClient.ProtocolMappers, newClient.ProtocolMappers)

	// Merge the secret-rotation attribute into the existing attributes map,
	// preserving any Keycloak-managed attributes already present.
	if newClient.Attributes != nil {
		if existingClient.Attributes == nil {
			existingClient.Attributes = &map[string]interface{}{}
		}
		(*existingClient.Attributes)[SecretRotationClientAttribute] = (*newClient.Attributes)[SecretRotationClientAttribute]
		return existingClient
	}

	clearSecretRotationAttributes(existingClient)
	return existingClient
}

// clearSecretRotationAttributes removes the opt-in attribute and the rotation
// state from a client that opted out of rotation. A Keycloak client PUT only
// updates the attributes it contains, so omitting a key keeps it; an empty
// value is what makes Keycloak remove the attribute.
func clearSecretRotationAttributes(c *api.ClientRepresentation) {
	if c.Attributes == nil {
		return
	}
	attrs := *c.Attributes
	for _, key := range append([]string{SecretRotationClientAttribute}, secretRotationStateAttributes...) {
		if _, ok := attrs[key]; ok {
			attrs[key] = ""
		}
	}
}
