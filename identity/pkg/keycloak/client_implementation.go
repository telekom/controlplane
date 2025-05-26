// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

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

	logger.V(1).Info("GetRealm", "ℹ️ request realm", realm)
	start := time.Now()
	getRealm, err := k.clientWithResponses.GetRealmWithResponse(ctx, realm)
	IncreaseDurationMetrics(start, "GET", "GetRealm")

	if err != nil {
		IncreaseErrorMetrics()
		return nil, err
	}

	if responseErr := CheckStatusCode(getRealm, http.StatusOK, http.StatusNotFound); responseErr != nil {
		IncreaseErrorMetrics()
		return nil, fmt.Errorf("❌ failed to get realm: %d -- Response for GET is: %s",
			getRealm.StatusCode(), string(getRealm.Body))
	}
	IncreaseStatusMetrics(strconv.Itoa(getRealm.StatusCode()), "GET", "GetRealm")

	logger.V(1).Info("GetRealm", "ℹ️ response", getRealm.JSON2XX)
	return getRealm, nil
}

func checkForRealmChanges(realm *identityv1.Realm,
	existingRealmRepresentation *api.RealmRepresentation,
	logger logr.Logger) *api.RealmRepresentation {
	realmRepresentation := mapper.MapToRealmRepresentation(realm)
	if mapper.CompareRealmRepresentation(existingRealmRepresentation, &realmRepresentation) {
		var message = fmt.Sprintf("ℹ️ No changes detected for realm %s with ID %d",
			realm.Name,
			existingRealmRepresentation.Id)
		logger.V(1).Info(message)
		return &realmRepresentation
	} else {
		var message = fmt.Sprintf("🧹 Changes found for realm %s in keycloak with ID %d\"",
			realm.Name, existingRealmRepresentation.Id)
		logger.V(1).Info(message)
		return mapper.MergeRealmRepresentation(existingRealmRepresentation, &realmRepresentation)
	}
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
		var message = fmt.Sprintf("🔍 Realm with name %s not found", realmName)
		logger.V(1).Info(message)
		return nil, fmt.Errorf("❌ realm to update does not exist")
	}

	// Check if there are any changes for the realm
	body := checkForRealmChanges(realm, existingRealmRepresentation, logger)

	logger.V(1).Info("PutRealm", "ℹ️ request realm", realmName)
	logger.V(1).Info("PutRealm", "ℹ️ request body", body)
	start := time.Now()
	put, err := k.clientWithResponses.PutRealmWithResponse(ctx, realmName, *body)

	IncreaseDurationMetrics(start, "PUT", "PutRealm")
	if err != nil {
		IncreaseErrorMetrics()
		return nil, err
	}

	if responseErr := CheckStatusCode(put, http.StatusNoContent); responseErr != nil {
		IncreaseErrorMetrics()
		return nil, fmt.Errorf("❌ failed to update realm: %d -- Response for PUT is: %s",
			put.StatusCode(), string(put.Body))
	}

	IncreaseStatusMetrics(strconv.Itoa(put.StatusCode()), "PUT", "PutRealm")
	logger.V(1).Info("PutRealm", "ℹ️ response", put.HTTPResponse.Body)
	return put, nil
}

func (k *realmClient) PostRealm(ctx context.Context, realm *identityv1.Realm) (*api.PostResponse, error) {
	logger := log.FromContext(ctx)
	if k.clientWithResponses == nil {
		return nil, fmt.Errorf("keycloak client is required")
	}

	body := mapper.MapToRealmRepresentation(realm)

	logger.V(1).Info("PostRealm", "️ℹ️ request body", body)
	start := time.Now()
	post, err := k.clientWithResponses.PostWithResponse(ctx, body)

	IncreaseDurationMetrics(start, "POST", "PostRealm")
	if err != nil {
		IncreaseErrorMetrics()
		return nil, err
	}

	if responseErr := CheckStatusCode(post, http.StatusCreated); responseErr != nil {
		IncreaseErrorMetrics()
		return nil, fmt.Errorf("❌ failed to create realm: %d -- Response for POST is: %s",
			post.StatusCode(), string(post.Body))
	}

	IncreaseStatusMetrics(strconv.Itoa(post.StatusCode()), "POST", "PostRealm")
	logger.V(1).Info("PostRealm", "ℹ️ response", post.HTTPResponse.Body)
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
		putRealm, responseErr := k.PutRealm(ctx, realm.Name, realm)
		if responseErr != nil {
			return responseErr
		}
		logger.V(1).Info("updated existing realm in keycloak", "realm", putRealm.Body)
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
		return fmt.Errorf("failed to delete realm: %s", string(response.Body))
	}

	var successMessage = fmt.Sprintf("✅ deleted realm %s", realm)
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

	logger.V(1).Info("GetRealmClients", "ℹ️ request realm", realm)
	logger.V(1).Info("GetRealmClients", "ℹ️ request params", getRealmClientsParams)

	start := time.Now()
	get, err := k.clientWithResponses.GetRealmClientsWithResponse(ctx, realm, getRealmClientsParams)
	IncreaseDurationMetrics(start, "GET", "GetRealmClients")
	if err != nil {
		IncreaseErrorMetrics()
		return nil, err
	}

	if responseErr := CheckStatusCode(get, http.StatusOK, http.StatusNotFound); responseErr != nil {
		IncreaseErrorMetrics()
		return nil, fmt.Errorf("❌ failed to list clients: %d -- Response for GET is: %s",
			get.StatusCode(), string(get.Body))
	}

	IncreaseStatusMetrics(strconv.Itoa(get.StatusCode()), "GET", "GetRealmClients")
	logger.V(1).Info("GetRealmClients", "response", ObfuscateClients(get.JSON2XX))
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
		return nil, fmt.Errorf("❌ failed to get client")
	}
}

func CheckForClientChanges(client *identityv1.Client,
	id string,
	existingClient *api.ClientRepresentation,
	logger logr.Logger) *api.ClientRepresentation {

	clientRepresentation := mapper.MapToClientRepresentation(client)
	if mapper.CompareClientRepresentation(existingClient, &clientRepresentation) {
		var message = fmt.Sprintf("ℹ️ No changes detected client %s with ID %s", client.Spec.ClientId, id)
		logger.V(1).Info(message)
		return &clientRepresentation
	} else {
		var message = fmt.Sprintf("🧹 Changes found for client %s in keycloak with ID %s\"",
			client.Spec.ClientId, id)
		logger.V(1).Info(message)
	}
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
		var message = fmt.Sprintf("🔍 RealmClient with ID %s not found", id)
		logger.V(1).Info(message)
		return nil, fmt.Errorf("client to update does not exist")
	}

	// Check if there are any changes to the realm client
	body := CheckForClientChanges(client, id, existingClient, logger)
	logger.V(1).Info("PutRealmClient", "ℹ️ request realm", realmName)
	logger.V(1).Info("PutRealmClient", "ℹ️ request ID", id)
	logger.V(1).Info("PutRealmClient", "ℹ️ request clientId", client.Spec.ClientId)

	start := time.Now()
	put, err := k.clientWithResponses.PutRealmClientsIdWithResponse(ctx, realmName, id, *body)

	IncreaseDurationMetrics(start, "PUT", "PutRealmClients")
	if err != nil {
		IncreaseErrorMetrics()
		return nil, err
	}

	if responseErr := CheckStatusCode(put, http.StatusNoContent); responseErr != nil {
		IncreaseErrorMetrics()
		return nil, fmt.Errorf("❌ failed to update client: %d -- Response for PUT is: %s",
			put.StatusCode(), string(put.Body))
	}

	IncreaseStatusMetrics(strconv.Itoa(put.StatusCode()), "PUT", "PutRealmClients")
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

	logger.V(1).Info("PostRealmClient", "ℹ️ request realm", realmName)
	logger.V(1).Info("PostRealmClient", "ℹ️ request clientId", client.Spec.ClientId)

	start := time.Now()
	post, err := k.clientWithResponses.PostRealmClientsWithResponse(ctx, realmName, body)
	IncreaseDurationMetrics(start, "POST", "PostRealmClients")
	if err != nil {
		IncreaseErrorMetrics()
		return nil, err
	}

	if responseErr := CheckStatusCode(post, http.StatusCreated); responseErr != nil {
		IncreaseErrorMetrics()
		return nil, fmt.Errorf("❌ failed to create client: %d -- Response for POST is: %s",
			post.StatusCode(), string(post.Body))
	}

	IncreaseStatusMetrics(strconv.Itoa(post.StatusCode()), "POST", "PostRealmClients")
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
		var message = fmt.Sprintf("🔍 found existing client %s in keycloak with ID %s",
			client.Spec.ClientId, *existingClient.Id)
		logger.V(1).Info(message, "client", existingClient)
		putRealmClient, err := k.PutRealmClient(ctx, realm.Name, *existingClient.Id, client)
		if err != nil {
			return err
		}
		var successMessage = fmt.Sprintf("✅ updated existing client %s in realm %s", client.Spec.ClientId, realm.Name)
		logger.V(1).Info(successMessage, "client", putRealmClient.Body)
	} else {
		var message = fmt.Sprintf("client %s not found in keycloak", client.Spec.ClientId)
		logger.V(1).Info(message)
		postRealmClient, err := k.PostRealmClient(ctx, realm.Name, client)
		if err != nil {
			return err
		}
		var successMessage = fmt.Sprintf("✅ created client %s in realm %s", client.Spec.ClientId, realm.Name)
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
		return fmt.Errorf("failed to delete realm client: %s", string(response.Body))
	}

	var successMessage = fmt.Sprintf("✅ deleted client %s in realm %s", id, realmName)
	logger.V(1).Info(successMessage, "client", response.Body)
	return nil
}

func ObfuscateClients(clients *[]api.ClientRepresentation) *[]api.ClientRepresentation {
	// Create a copy of the status to avoid modifying the original
	obfuscatedClients := *clients

	// Obfuscate sensitive fields
	for i := range obfuscatedClients {
		if *obfuscatedClients[i].Secret != "" {
			obfuscatedClients[i].Secret = ptr.To("****")
		}
	}

	return &obfuscatedClients
}
