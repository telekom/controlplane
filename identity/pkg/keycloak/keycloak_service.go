// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
	CreateOrReplaceClient(ctx context.Context, realmName string, client *identityv1.Client, supportsGracefulRotation bool) error
	DeleteClient(ctx context.Context, realmName string, client *identityv1.Client) error

	// Secret rotation
	// GetClientSecretRotationInfo returns the secret rotation state for a
	// Keycloak client. It always returns a non-nil *ClientSecretRotationInfo
	// when the client exists. The struct contains:
	//   - RotatedSecret / RotatedCreatedAt / RotatedExpiresAt when a rotation
	//     grace period is active (empty/nil otherwise).
	//   - SecretCreationTime: epoch-seconds of the current secret's creation
	//     (nil when the attribute is missing).
	// Returns (nil, err) on communication or unexpected errors.
	GetClientSecretRotationInfo(ctx context.Context, realmName string, client *identityv1.Client) (*ClientSecretRotationInfo, error)
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
			log.V(2).Info("client found by uuid", "response", string(clientRes.Body))
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
	if responseErr := CheckHTTPStatus(clientRes.StatusCode(), 200); responseErr != nil {
		return nil, fmt.Errorf("fetching client by ClientId: %w", responseErr)
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
func (k *keycloakService) CreateOrReplaceClient(ctx context.Context, realmName string, client *identityv1.Client, supportsGracefulRotation bool) error {
	existing, err := k.getClient(ctx, realmName, client)
	if err != nil {
		return fmt.Errorf("error checking for existing client: %w", err)
	}

	if existing == nil {
		return k.createClient(ctx, realmName, client)
	}

	return k.updateClient(ctx, realmName, existing, client, supportsGracefulRotation)
}

func (k *keycloakService) createClient(ctx context.Context, realmName string, client *identityv1.Client) error {
	logger := logr.FromContextOrDiscard(ctx)

	body := util.MapToClientRepresentation(client)
	res, err := k.Client.PostRealmClientsWithResponse(ctx, realmName, body)
	if err != nil {
		return fmt.Errorf("error creating client: %w", err)
	}
	if responseErr := CheckHTTPStatus(res.StatusCode(), 201); responseErr != nil {
		return fmt.Errorf("creating client: %w", responseErr)
	}

	clientUid, err := resourceIDFromResponse(res.HTTPResponse)
	if err != nil {
		return fmt.Errorf("client was created but failed to extract ID: %w", err)
	}

	client.Status.ClientUid = clientUid
	logger.V(1).Info("created client in keycloak", "clientId", client.Spec.ClientId, "uid", client.Status.ClientUid)
	return nil
}

// resourceIDFromResponse extracts the resource UUID from the Location header
// returned by Keycloak on resource creation (HTTP 201). The UUID is the last
// path segment of the Location URL.
func resourceIDFromResponse(resp *http.Response) (string, error) {
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("response did not contain a Location header")
	}
	locURL, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("failed to parse Location header %q: %w", location, err)
	}
	segments := strings.Split(strings.TrimRight(locURL.Path, "/"), "/")
	id := segments[len(segments)-1]
	if id == "" {
		return "", fmt.Errorf("Location header %q did not contain a resource ID", location)
	}
	return id, nil
}

// updateClient compares the desired state with the existing Keycloak
// representation. When differences are detected the existing object is merged
// with the desired fields (preserving Keycloak-managed attributes such as Id,
// DefaultClientScopes, Attributes, etc.) and PUT back. If the client secret
// changed and the realm supports graceful rotation, a forced secret rotation is
// triggered first so that Keycloak's rotation executor preserves the old secret
// in the "rotated" slot.
func (k *keycloakService) updateClient(ctx context.Context, realmName string, existing *api.ClientRepresentation, client *identityv1.Client, supportsGracefulRotation bool) error {
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

	// If the secret changed and the realm supports graceful rotation, force
	// rotation before PUT so Keycloak moves the current secret into the
	// "rotated" slot with the configured grace period.
	if util.HasSecretChanged(existing, &desired) && supportsGracefulRotation {
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
			return fmt.Errorf("re-fetching client after rotation: %w", CheckHTTPStatus(refreshed.StatusCode(), 200))
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
	if responseErr := CheckHTTPStatus(res.StatusCode(), 200, 204); responseErr != nil {
		return fmt.Errorf("updating client: %w", responseErr)
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
		if responseErr := CheckHTTPStatus(res.StatusCode(), 204); responseErr != nil {
			return fmt.Errorf("updating realm: %w", responseErr)
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
	if responseErr := CheckHTTPStatus(res.StatusCode(), 201); responseErr != nil {
		return fmt.Errorf("creating realm: %w", responseErr)
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
	if responseErr := CheckHTTPStatus(res.StatusCode(), 204); responseErr != nil {
		return fmt.Errorf("deleting client: %w", responseErr)
	}

	return nil
}

// DeleteRealm implements [KeycloakService].
func (k *keycloakService) DeleteRealm(ctx context.Context, realmName string) error {
	res, err := k.Client.DeleteRealmWithResponse(ctx, realmName)
	if err != nil {
		return fmt.Errorf("error deleting realm: %w", err)
	}
	if responseErr := CheckHTTPStatus(res.StatusCode(), 204, 404); responseErr != nil {
		return fmt.Errorf("deleting realm: %w", responseErr)
	}

	return nil
}

func NewKeycloakService(client KeycloakClient) KeycloakService {
	return &keycloakService{
		Client: client,
	}
}
