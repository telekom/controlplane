// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak/mapper"
)

var _ RealmClient = &realmClient{}

type realmClient struct {
	clientWithResponses api.KeycloakClient
}

var NewRealmClient = func(client api.KeycloakClient) RealmClient {
	return &realmClient{
		clientWithResponses: client,
	}
}

func (k *realmClient) GetRealm(ctx context.Context, realm string) (*api.GetRealmResponse, error) {
	logger := log.FromContext(ctx)
	if k.clientWithResponses == nil {
		return nil, fmt.Errorf("keycloak client is required")
	}

	logger.V(1).Info("GetRealm", "request realm", realm)
	getRealm, err := k.clientWithResponses.GetRealmWithResponse(ctx, realm)

	if err != nil {
		return nil, err
	}

	if responseErr := CheckStatusCode(getRealm, http.StatusOK, http.StatusNotFound); responseErr != nil {
		return nil, fmt.Errorf("failed to get realm: %d -- Response for GET is: %s: %w",
			getRealm.StatusCode(), string(getRealm.Body), responseErr)
	}

	logger.V(1).Info("GetRealm", "response", getRealm.JSON2XX)
	return getRealm, nil
}

func checkForRealmChanges(realm *identityv1.Realm,
	existingRealmRepresentation *api.RealmRepresentation,
	logger logr.Logger) *api.RealmRepresentation {
	realmRepresentation := mapper.MapToRealmRepresentation(realm)
	if mapper.CompareRealmRepresentation(existingRealmRepresentation, &realmRepresentation) {
		var message = fmt.Sprintf("No changes detected for realm %s with ID %d",
			realm.Name,
			existingRealmRepresentation.Id)
		logger.V(1).Info(message)
		return nil
	}
	var message = fmt.Sprintf("Changes found for realm %s in keycloak with ID %d",
		realm.Name, existingRealmRepresentation.Id)
	logger.V(1).Info(message)
	return mapper.MergeRealmRepresentation(existingRealmRepresentation, &realmRepresentation)
}

func (k *realmClient) PutRealm(ctx context.Context, realmName string,
	realm *identityv1.Realm) (*api.PutRealmResponse, error) {
	logger := log.FromContext(ctx)
	if k.clientWithResponses == nil {
		return nil, fmt.Errorf("keycloak client is required")
	}

	// Get existing realm
	var existingRealm, err = k.GetRealm(ctx, realmName)
	if err != nil {
		return nil, err
	}
	var existingRealmRepresentation = existingRealm.JSON2XX
	if existingRealmRepresentation == nil {
		logger.V(1).Info("Realm not found for update", "realm", realmName)
		return nil, fmt.Errorf("realm to update does not exist")
	}

	return k.putRealmWithExisting(ctx, realmName, realm, existingRealmRepresentation)
}

// putRealmWithExisting performs the PUT using an already-fetched representation,
// avoiding a redundant GET when called from CreateOrUpdateRealm.
func (k *realmClient) putRealmWithExisting(ctx context.Context, realmName string,
	realm *identityv1.Realm, existing *api.RealmRepresentation) (*api.PutRealmResponse, error) {
	logger := log.FromContext(ctx)

	// Check if there are any changes for the realm
	body := checkForRealmChanges(realm, existing, logger)
	if body == nil {
		// No changes detected — skip the PUT
		return nil, nil
	}

	logger.V(1).Info("PutRealm", "request realm", realmName)
	logger.V(1).Info("PutRealm", "request body", body)
	put, err := k.clientWithResponses.PutRealmWithResponse(ctx, realmName, *body)

	if err != nil {
		return nil, err
	}

	if responseErr := CheckStatusCode(put, http.StatusNoContent); responseErr != nil {
		return nil, fmt.Errorf("failed to update realm: %d -- Response for PUT is: %s: %w",
			put.StatusCode(), string(put.Body), responseErr)
	}

	logger.V(1).Info("PutRealm", "response", put.HTTPResponse.Body)
	return put, nil
}

func (k *realmClient) PostRealm(ctx context.Context, realm *identityv1.Realm) (*api.PostResponse, error) {
	logger := log.FromContext(ctx)
	if k.clientWithResponses == nil {
		return nil, fmt.Errorf("keycloak client is required")
	}

	body := mapper.MapToRealmRepresentation(realm)

	logger.V(1).Info("PostRealm", "️request body", body)
	post, err := k.clientWithResponses.PostWithResponse(ctx, body)

	if err != nil {
		return nil, err
	}

	if responseErr := CheckStatusCode(post, http.StatusCreated); responseErr != nil {
		return nil, fmt.Errorf("failed to create realm: %d -- Response for POST is: %s: %w",
			post.StatusCode(), string(post.Body), responseErr)
	}

	logger.V(1).Info("PostRealm", "response", post.HTTPResponse.Body)
	return post, nil
}

func (k *realmClient) CreateOrUpdateRealm(ctx context.Context, realm *identityv1.Realm) error {
	logger := log.FromContext(ctx)

	getRealm, err := k.GetRealm(ctx, realm.Name)
	if err != nil {
		return err
	}

	if getRealm.StatusCode() == http.StatusOK {
		logger.V(1).Info("found existing realm in keycloak", "realm", getRealm.Body)
		// Reuse the already-fetched representation to avoid a redundant GET inside PutRealm
		existing := getRealm.JSON2XX
		if existing == nil {
			return fmt.Errorf("realm to update does not exist")
		}
		putRealm, responseErr := k.putRealmWithExisting(ctx, realm.Name, realm, existing)
		if responseErr != nil {
			return responseErr
		}
		if putRealm != nil {
			logger.V(1).Info("updated existing realm in keycloak", "realm", putRealm.Body)
		} else {
			logger.V(1).Info("no changes detected for realm in keycloak, skipping update")
		}
	} else {
		logger.V(1).Info("realm not found in keycloak", "realm", getRealm.Body)
		postRealm, responseErr := k.PostRealm(ctx, realm)
		if responseErr != nil {
			return responseErr
		}
		logger.V(1).Info("created realm in keycloak", "realm", postRealm.Body)
	}

	return nil
}

func (k *realmClient) DeleteRealm(ctx context.Context, realm string) error {
	logger := log.FromContext(ctx)

	response, err := k.clientWithResponses.DeleteRealmWithResponse(ctx, realm)
	if err != nil {
		return err
	}
	if err := CheckStatusCode(response, 200, 204, 404); err != nil {
		return fmt.Errorf("failed to delete realm: %s: %w", string(response.Body), err)
	}

	var successMessage = fmt.Sprintf("deleted realm %s", realm)
	logger.V(1).Info(successMessage, "realm", response.Body)

	return nil
}

func (k *realmClient) GetRealmClients(ctx context.Context, realm string,
	client *identityv1.Client) (*api.GetRealmClientsResponse, error) {
	logger := log.FromContext(ctx)
	if k.clientWithResponses == nil {
		return nil, fmt.Errorf("keycloak client is required")
	}

	var getRealmClientsParams = &api.GetRealmClientsParams{
		ClientId:     ptr.To(client.Spec.ClientId),
		Search:       ptr.To(false), // Exact search only
		ViewableOnly: ptr.To(true),
	}

	logger.V(1).Info("GetRealmClients", "request realm", realm)
	logger.V(1).Info("GetRealmClients", "request params", getRealmClientsParams)

	get, err := k.clientWithResponses.GetRealmClientsWithResponse(ctx, realm, getRealmClientsParams)
	if err != nil {
		return nil, err
	}

	if responseErr := CheckStatusCode(get, http.StatusOK, http.StatusNotFound); responseErr != nil {
		return nil, fmt.Errorf("failed to list clients: %d -- Response for GET is: %s: %w",
			get.StatusCode(), string(get.Body), responseErr)
	}

	return get, nil
}

func (k *realmClient) getRealmClient(ctx context.Context, realmName string,
	client *identityv1.Client) (*api.ClientRepresentation, error) {
	var getRealmClients, err = k.GetRealmClients(ctx, realmName, client)
	if err != nil {
		return nil, err
	}

	if getRealmClients.StatusCode() == http.StatusOK {
		existingClient, getErr := mapper.GetClient(*getRealmClients)
		if getErr != nil {
			return nil, getErr
		}
		return existingClient, nil
	} else {
		return nil, fmt.Errorf("failed to get client")
	}
}

func CheckForClientChanges(client *identityv1.Client,
	id string,
	existingClient *api.ClientRepresentation,
	logger logr.Logger) *api.ClientRepresentation {

	clientRepresentation := mapper.MapToClientRepresentation(client)
	if mapper.CompareClientRepresentation(existingClient, &clientRepresentation) {
		var message = fmt.Sprintf("No changes detected client %s with ID %s", client.Spec.ClientId, id)
		logger.V(1).Info(message)
		return nil
	}
	var message = fmt.Sprintf("Changes found for client %s in keycloak with ID %s",
		client.Spec.ClientId, id)
	logger.V(1).Info(message)
	// Merge existing realm client with new realm client and update it in keycloak
	return mapper.MergeClientRepresentation(existingClient, &clientRepresentation)
}

func (k *realmClient) PutRealmClient(ctx context.Context, realmName, id string,
	client *identityv1.Client) (*api.PutRealmClientsIdResponse, error) {
	logger := log.FromContext(ctx)
	if k.clientWithResponses == nil {
		return nil, fmt.Errorf("keycloak client is required")
	}

	// Get existing realm client
	var existingClient, err = k.getRealmClient(ctx, realmName, client)
	if err != nil {
		return nil, err
	}
	if existingClient == nil {
		logger.V(1).Info("RealmClient not found for update", "id", id)
		return nil, fmt.Errorf("client to update does not exist")
	}

	return k.putRealmClientWithExisting(ctx, realmName, id, client, existingClient)
}

// putRealmClientWithExisting performs the PUT using an already-fetched representation,
// avoiding a redundant GET when called from CreateOrUpdateRealmClient.
func (k *realmClient) putRealmClientWithExisting(ctx context.Context, realmName, id string,
	client *identityv1.Client, existing *api.ClientRepresentation) (*api.PutRealmClientsIdResponse, error) {
	logger := log.FromContext(ctx)

	// Check if there are any changes to the realm client
	body := CheckForClientChanges(client, id, existing, logger)
	if body == nil {
		// No changes detected — skip the PUT
		return nil, nil
	}
	logger.V(1).Info("PutRealmClient", "request realm", realmName)
	logger.V(1).Info("PutRealmClient", "request ID", id)
	logger.V(1).Info("PutRealmClient", "request clientId", client.Spec.ClientId)

	put, err := k.clientWithResponses.PutRealmClientsIdWithResponse(ctx, realmName, id, *body)

	if err != nil {
		return nil, err
	}

	if responseErr := CheckStatusCode(put, http.StatusNoContent); responseErr != nil {
		return nil, fmt.Errorf("failed to update client: %d -- Response for PUT is: %s: %w",
			put.StatusCode(), string(put.Body), responseErr)
	}

	logger.V(1).Info("PutRealmClient", "response", put.HTTPResponse.Body)
	return put, nil
}

func (k *realmClient) PostRealmClient(ctx context.Context, realmName string,
	client *identityv1.Client) (*api.PostRealmClientsResponse, error) {
	logger := log.FromContext(ctx)
	if k.clientWithResponses == nil {
		return nil, fmt.Errorf("keycloak client is required")
	}

	body := mapper.MapToClientRepresentation(client)

	logger.V(1).Info("PostRealmClient", "request realm", realmName)
	logger.V(1).Info("PostRealmClient", "request clientId", client.Spec.ClientId)

	post, err := k.clientWithResponses.PostRealmClientsWithResponse(ctx, realmName, body)
	if err != nil {
		return nil, err
	}

	if responseErr := CheckStatusCode(post, http.StatusCreated); responseErr != nil {
		return nil, fmt.Errorf("failed to create client: %d -- Response for POST is: %s: %w",
			post.StatusCode(), string(post.Body), responseErr)
	}

	logger.V(1).Info("PostRealmClient", "response", post.HTTPResponse.Body)
	return post, nil
}

func (k *realmClient) CreateOrUpdateRealmClient(ctx context.Context, realm *identityv1.Realm,
	client *identityv1.Client) error {
	logger := log.FromContext(ctx)

	var existingClient, err = k.getRealmClient(ctx, realm.Name, client)
	if err != nil {
		return err
	}

	if existingClient != nil && existingClient.Id != nil && *existingClient.Id != "" {
		logger.V(1).Info("found existing client in keycloak",
			"clientId", client.Spec.ClientId, "id", *existingClient.Id)
		// Reuse the already-fetched representation to avoid a redundant GET inside PutRealmClient
		putRealmClient, err := k.putRealmClientWithExisting(ctx, realm.Name, *existingClient.Id, client, existingClient)
		if err != nil {
			return err
		}
		if putRealmClient != nil {
			var successMessage = fmt.Sprintf("updated existing client %s in realm %s", client.Spec.ClientId, realm.Name)
			logger.V(1).Info(successMessage, "client", putRealmClient.Body)
		} else {
			logger.V(1).Info("no changes detected for client, skipping update",
				"clientId", client.Spec.ClientId, "realm", realm.Name)
		}
	} else {
		var message = fmt.Sprintf("client %s not found in keycloak", client.Spec.ClientId)
		logger.V(1).Info(message)
		postRealmClient, err := k.PostRealmClient(ctx, realm.Name, client)
		if err != nil {
			return err
		}
		var successMessage = fmt.Sprintf("created client %s in realm %s", client.Spec.ClientId, realm.Name)
		logger.V(1).Info(successMessage, "client", postRealmClient.Body)
	}

	return nil
}

func (k *realmClient) DeleteRealmClient(ctx context.Context, realmName string, id string) error {
	logger := log.FromContext(ctx)
	response, err := k.clientWithResponses.DeleteRealmClientsIdWithResponse(ctx, realmName, id)
	if err != nil {
		return err
	}
	if err := CheckStatusCode(response, 200, 204, 404); err != nil {
		return fmt.Errorf("failed to delete realm client: %s: %w", string(response.Body), err)
	}

	var successMessage = fmt.Sprintf("deleted client %s in realm %s", id, realmName)
	logger.V(1).Info(successMessage, "client", response.Body)
	return nil
}
