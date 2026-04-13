// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak/util"
)

type KeycloakClient interface {
	// Realm operations
	GetRealmWithResponse(ctx context.Context, realm string,
		reqEditors ...api.RequestEditorFn) (*api.GetRealmResponse, error)
	PostWithResponse(ctx context.Context, body api.PostJSONRequestBody,
		reqEditors ...api.RequestEditorFn) (*api.PostResponse, error)
	PutRealmWithResponse(ctx context.Context, realm string, body api.PutRealmJSONRequestBody,
		reqEditors ...api.RequestEditorFn) (*api.PutRealmResponse, error)
	DeleteRealmWithResponse(ctx context.Context, realm string,
		reqEditors ...api.RequestEditorFn) (*api.DeleteRealmResponse, error)

	// Client CRUD operations
	GetRealmClientsWithResponse(ctx context.Context, realm string, params *api.GetRealmClientsParams,
		reqEditors ...api.RequestEditorFn) (*api.GetRealmClientsResponse, error)
	GetRealmClientsIdWithResponse(ctx context.Context, realm string, id string,
		reqEditors ...api.RequestEditorFn) (*api.GetRealmClientsIdResponse, error)
	PostRealmClientsWithResponse(ctx context.Context, realm string, body api.PostRealmClientsJSONRequestBody,
		reqEditors ...api.RequestEditorFn) (*api.PostRealmClientsResponse, error)
	PutRealmClientsIdWithResponse(ctx context.Context, realm string, id string, body api.PutRealmClientsIdJSONRequestBody,
		reqEditors ...api.RequestEditorFn) (*api.PutRealmClientsIdResponse, error)
	DeleteRealmClientsIdWithResponse(ctx context.Context, realm string, id string,
		reqEditors ...api.RequestEditorFn) (*api.DeleteRealmClientsIdResponse, error)

	// Secret rotation operations
	// PostRealmClientsIdClientSecretWithResponse regenerates the client secret.
	// When the secret-rotation executor is active, this moves the current
	// secret into the "rotated" slot with the configured grace period.
	PostRealmClientsIdClientSecretWithResponse(ctx context.Context, realm string, id string,
		reqEditors ...api.RequestEditorFn) (*api.PostRealmClientsIdClientSecretResponse, error)

	DeleteRealmClientsIdClientSecretRotatedWithResponse(ctx context.Context, realm string, id string,
		reqEditors ...api.RequestEditorFn) (*api.DeleteRealmClientsIdClientSecretRotatedResponse, error)

	// GetRealmClientsIdClientSecretRotatedWithResponse retrieves the rotated
	// (old) client secret during a graceful rotation grace period.
	GetRealmClientsIdClientSecretRotatedWithResponse(ctx context.Context, realm string, id string,
		reqEditors ...api.RequestEditorFn) (*api.GetRealmClientsIdClientSecretRotatedResponse, error)

	// Client policies operations (secret rotation policy configuration)
	GetRealmClientPoliciesPoliciesWithResponse(ctx context.Context, realm string, params *api.GetRealmClientPoliciesPoliciesParams,
		reqEditors ...api.RequestEditorFn) (*api.GetRealmClientPoliciesPoliciesResponse, error)
	PutRealmClientPoliciesPoliciesWithResponse(ctx context.Context, realm string, body api.PutRealmClientPoliciesPoliciesJSONRequestBody,
		reqEditors ...api.RequestEditorFn) (*api.PutRealmClientPoliciesPoliciesResponse, error)
	GetRealmClientPoliciesProfilesWithResponse(ctx context.Context, realm string, params *api.GetRealmClientPoliciesProfilesParams,
		reqEditors ...api.RequestEditorFn) (*api.GetRealmClientPoliciesProfilesResponse, error)
	PutRealmClientPoliciesProfilesWithResponse(ctx context.Context, realm string, body api.PutRealmClientPoliciesProfilesJSONRequestBody,
		reqEditors ...api.RequestEditorFn) (*api.PutRealmClientPoliciesProfilesResponse, error)
}

type KeycloakService interface {
	// Realm operations
	CreateOrReplaceRealm(ctx context.Context, realm *identityv1.Realm) error
	DeleteRealm(ctx context.Context, realmName string) error
	ConfigureSecretRotationPolicy(ctx context.Context, realmName string, policy *identityv1.SecretRotationConfig) error

	// Client operations
	CreateOrReplaceClient(ctx context.Context, realmName string, client *identityv1.Client) error
	DeleteClient(ctx context.Context, realmName string, client *identityv1.Client) error

	// Secret rotation
	// GetRotatedClientSecret returns the rotated (old) client secret for a
	// Keycloak client during a graceful rotation grace period.
	// Returns:
	//   - (*RotatedSecretInfo, nil) when a rotated secret exists (rotation in progress).
	//     Secret is the plaintext value; CreatedAt and ExpiresAt are epoch-second
	//     timestamps from Keycloak's client attributes (may be nil if unavailable).
	//   - (nil, nil) when no rotated secret exists (no rotation / grace period expired)
	//   - (nil, err) on communication or unexpected errors
	GetRotatedClientSecret(ctx context.Context, realmName string, client *identityv1.Client) (*RotatedSecretInfo, error)
}

var _ KeycloakService = (*keycloakService)(nil)

type keycloakService struct {
	Client KeycloakClient
}

func (k *keycloakService) getClient(ctx context.Context, realmName string, client *identityv1.Client) (*api.ClientRepresentation, error) {
	clientId := client.Spec.ClientId
	clientUid := client.Status.ClientUid

	log := logr.FromContextOrDiscard(ctx)

	if len(clientUid) > 0 {
		clientRes, err := k.Client.GetRealmClientsIdWithResponse(ctx, realmName, clientUid)
		if err != nil {
			return nil, fmt.Errorf("unexpected error when fetching client by UID: %w", err)
		}
		if clientRes.StatusCode() == 200 && clientRes.JSON2XX != nil {
			log.V(1).Info("client found by uuid", "response", string(clientRes.Body))
			return clientRes.JSON2XX, nil
		}
		// UID lookup returned 404 or unexpected status — fall through to clientId search
	}

	noSearch := false
	onlyViewable := true
	params := api.GetRealmClientsParams{
		ClientId:     &clientId,
		Search:       &noSearch,
		ViewableOnly: &onlyViewable,
	}
	clientRes, err := k.Client.GetRealmClientsWithResponse(ctx, realmName, &params)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when fetching client by ClientId: %w", err)
	}
	if clientRes.StatusCode() == 404 {
		return nil, nil
	}
	if clientRes.StatusCode() != 200 {
		return nil, fmt.Errorf("unexpected status code %d when fetching client by ClientId", clientRes.StatusCode())
	}
	if clientRes.JSON2XX == nil {
		return nil, fmt.Errorf("unexpected empty response body when fetching client by ClientId")
	}

	switch len(*clientRes.JSON2XX) {
	case 0:
		return nil, nil
	case 1:
		log.V(1).Info("client found by clientId", "response", string(clientRes.Body))
		return &(*clientRes.JSON2XX)[0], nil
	default:
		return nil, fmt.Errorf("multiple clients found with ClientId %s", clientId)
	}
}

// CreateOrReplaceClient implements [KeycloakService].
func (k *keycloakService) CreateOrReplaceClient(ctx context.Context, realmName string, client *identityv1.Client) error {
	existing, err := k.getClient(ctx, realmName, client)
	if err != nil {
		return fmt.Errorf("error checking for existing client: %w", err)
	}

	if existing == nil {
		return k.createClient(ctx, realmName, client)
	}

	return k.updateClient(ctx, realmName, existing, client)
}

func (k *keycloakService) createClient(ctx context.Context, realmName string, client *identityv1.Client) error {
	logger := logr.FromContextOrDiscard(ctx)

	body := util.MapToClientRepresentation(client)
	res, err := k.Client.PostRealmClientsWithResponse(ctx, realmName, body)
	if err != nil {
		return fmt.Errorf("error creating client: %w", err)
	}
	if res.StatusCode() != 201 {
		return fmt.Errorf("unexpected status code %d when creating client", res.StatusCode())
	}

	var resBody api.ClientRepresentation
	if err := json.Unmarshal(res.Body, &resBody); err != nil {
		return fmt.Errorf("client was created but failed to parse response body: %w", err)
	}
	if resBody.Id == nil {
		return fmt.Errorf("client was created but response did not contain an ID")
	}

	client.Status.ClientUid = *resBody.Id
	logger.V(1).Info("created client in keycloak", "clientId", client.Spec.ClientId, "uid", client.Status.ClientUid)
	return nil
}

// updateClient compares the desired state with the existing Keycloak
// representation. When differences are detected the existing object is merged
// with the desired fields (preserving Keycloak-managed attributes such as Id,
// DefaultClientScopes, Attributes, etc.) and PUT back. If the client secret
// changed, a forced secret rotation is triggered first so that Keycloak's
// rotation executor preserves the old secret in the "rotated" slot.
func (k *keycloakService) updateClient(ctx context.Context, realmName string, existing *api.ClientRepresentation, client *identityv1.Client) error {
	logger := logr.FromContextOrDiscard(ctx)
	clientUUID := *existing.Id
	client.Status.ClientUid = clientUUID

	desired := util.MapToClientRepresentation(client)

	// Skip update when nothing changed.
	if util.CompareClientRepresentation(existing, &desired) {
		logger.V(1).Info("no changes detected for client, skipping update",
			"clientId", client.Spec.ClientId, "keycloakId", clientUUID)
		return nil
	}

	// If the secret changed, force rotation before PUT so Keycloak moves the
	// current secret into the "rotated" slot with the configured grace period.
	if util.HasSecretChanged(existing, &desired) {
		logger.V(1).Info("secret change detected, forcing rotation before update",
			"clientId", client.Spec.ClientId, "keycloakId", clientUUID)
		if err := k.forceSecretRotation(ctx, realmName, clientUUID); err != nil {
			return fmt.Errorf("failed to force secret rotation before updating client %s: %w",
				client.Spec.ClientId, err)
		}
		refreshed, err := k.Client.GetRealmClientsIdWithResponse(ctx, realmName, clientUUID)
		if err != nil {
			return fmt.Errorf("failed to re-fetch client after rotation: %w", err)
		}
		if refreshed.JSON2XX == nil {
			return fmt.Errorf("failed to re-fetch client after rotation: unexpected status %d", refreshed.StatusCode())
		}
		existing = refreshed.JSON2XX
	}

	// Merge desired fields into the existing representation and PUT.
	merged := util.MergeClientRepresentation(existing, &desired)
	body := *merged

	res, err := k.Client.PutRealmClientsIdWithResponse(ctx, realmName, clientUUID, body)
	if err != nil {
		return fmt.Errorf("error updating client: %w", err)
	}
	if res.StatusCode() != 204 && res.StatusCode() != 200 {
		return fmt.Errorf("unexpected status code %d when updating client", res.StatusCode())
	}

	logger.V(1).Info("updated existing client in keycloak",
		"clientId", client.Spec.ClientId, "keycloakId", clientUUID)
	return nil
}

// CreateOrReplaceRealm implements [KeycloakService].
//
// It checks whether the realm already exists in Keycloak. If it does, the
// desired state is compared with the existing representation using the mapper
// package; a PUT is issued only when differences are detected, and the merged
// representation preserves all Keycloak-managed fields. If the realm does not
// exist, it is created via POST.
func (k *keycloakService) CreateOrReplaceRealm(ctx context.Context, realm *identityv1.Realm) error {
	logger := logr.FromContextOrDiscard(ctx)

	getRealm, err := k.Client.GetRealmWithResponse(ctx, realm.Name)
	if err != nil {
		return fmt.Errorf("error checking for existing realm: %w", err)
	}

	if getRealm.StatusCode() == 200 && getRealm.JSON2XX != nil {
		// Realm exists — compare and merge before PUT.
		existing := getRealm.JSON2XX
		desired := util.MapToRealmRepresentation(realm)
		if util.CompareRealmRepresentation(existing, &desired) {
			logger.V(1).Info("no changes detected for realm, skipping update", "realm", realm.Name)
			return nil
		}
		merged := util.MergeRealmRepresentation(existing, &desired)
		body := *merged
		res, err := k.Client.PutRealmWithResponse(ctx, realm.Name, body)
		if err != nil {
			return fmt.Errorf("error updating realm: %w", err)
		}
		if res.StatusCode() != 204 {
			return fmt.Errorf("unexpected status %d when updating realm", res.StatusCode())
		}
		logger.V(1).Info("updated existing realm in keycloak", "realm", realm.Name)
		return nil
	}

	// Realm does not exist — create it.
	body := util.MapToRealmRepresentation(realm)
	res, err := k.Client.PostWithResponse(ctx, body)
	if err != nil {
		return fmt.Errorf("error creating realm: %w", err)
	}
	if res.StatusCode() != 201 {
		return fmt.Errorf("unexpected status %d when creating realm", res.StatusCode())
	}
	logger.V(1).Info("created realm in keycloak", "realm", realm.Name)
	return nil
}

// DeleteClient implements [KeycloakService].
func (k *keycloakService) DeleteClient(ctx context.Context, realmName string, client *identityv1.Client) error {
	keycloakClient, err := k.getClient(ctx, realmName, client)
	if err != nil {
		return fmt.Errorf("error checking for existing client: %w", err)
	}

	if keycloakClient == nil {
		// Client does not exist, nothing to do
		return nil
	}

	res, err := k.Client.DeleteRealmClientsIdWithResponse(ctx, realmName, *keycloakClient.Id)
	if err != nil {
		return fmt.Errorf("error deleting client: %w", err)
	}
	if res.StatusCode() != 204 {
		return fmt.Errorf("unexpected status code %d when deleting client", res.StatusCode())
	}

	return nil
}

// DeleteRealm implements [KeycloakService].
func (k *keycloakService) DeleteRealm(ctx context.Context, realmName string) error {
	res, err := k.Client.DeleteRealmWithResponse(ctx, realmName)
	if err != nil {
		return fmt.Errorf("error deleting realm: %w", err)
	}
	if res.StatusCode() != 204 && res.StatusCode() != 404 {
		return fmt.Errorf("unexpected status code %d when deleting realm", res.StatusCode())
	}

	return nil
}

func NewKeycloakService(client KeycloakClient) KeycloakService {
	return &keycloakService{
		Client: client,
	}
}
