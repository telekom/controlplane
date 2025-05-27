// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/test/mocks"
)

const (
	Realm                = "test-realm"
	RealmForClient       = "realm-test-client"
	RealmForEmpty        = "empty-realm"
	RealmForPut          = "put-test"
	RealmForClientPut    = "put-test-client"
	RealmForDelete       = "delete-test"
	RealmForClientDelete = "delete-test-client"
	ClientId             = "test-client"
	ClientSecret         = "test-secret"
)

func TestGetRealmReturnsClientError(t *testing.T) {
	mockClient := NewRealmClient(nil)
	result, err := mockClient.GetRealm(context.Background(), RealmForClient)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetRealmReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	result, err := mockClient.GetRealm(context.Background(), RealmForClient)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetRealmReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	result, err := mockClient.GetRealm(context.Background(), Realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPutRealmReturnsClientError(t *testing.T) {
	mockClient := NewRealmClient(nil)
	realm := &identityv1.Realm{}
	result, err := mockClient.PutRealm(context.Background(), RealmForPut, realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestDeleteRealmReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	err := mockClient.DeleteRealm(context.Background(), Realm)

	assert.Error(t, err)
}

func TestDeleteRealmReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	err := mockClient.DeleteRealm(context.Background(), Realm)

	assert.Error(t, err)
}

func TestPutRealmReturnsErrorForGet(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	realm := &identityv1.Realm{}
	result, err := mockClient.PutRealm(context.Background(), RealmForClient, realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPutRealmReturnsErrorForEmptyGet(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	realm := &identityv1.Realm{}
	result, err := mockClient.PutRealm(context.Background(), RealmForEmpty, realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPutRealmReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	realm := &identityv1.Realm{}
	result, err := mockClient.PutRealm(context.Background(), RealmForClientPut, realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPutRealmReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	realm := &identityv1.Realm{}
	result, err := mockClient.PutRealm(context.Background(), RealmForPut, realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPostRealmReturnsClientError(t *testing.T) {
	mockClient := NewRealmClient(nil)
	realm := &identityv1.Realm{}
	realm.Name = RealmForClient
	result, err := mockClient.PostRealm(context.Background(), realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPostRealmReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	realm := &identityv1.Realm{}
	realm.Name = RealmForClient
	result, err := mockClient.PostRealm(context.Background(), realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPostRealmReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	realm := &identityv1.Realm{}
	realm.Name = Realm
	result, err := mockClient.PostRealm(context.Background(), realm)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetRealmClientsReturnsClientError(t *testing.T) {
	mockClient := NewRealmClient(nil)
	client := &identityv1.Client{}
	result, err := mockClient.GetRealmClients(context.Background(), RealmForClient, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetRealmClientsReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	client := &identityv1.Client{}
	result, err := mockClient.GetRealmClients(context.Background(), RealmForClient, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetRealmClientsReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	client := &identityv1.Client{}
	result, err := mockClient.GetRealmClients(context.Background(), Realm, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestDeleteRealmClientReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	err := mockClient.DeleteRealmClient(context.Background(), Realm, RealmForClientDelete)
	assert.Error(t, err)

}

func TestDeleteRealmClientReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	err := mockClient.DeleteRealmClient(context.Background(), Realm, RealmForClientDelete)
	assert.Error(t, err)
}

func TestPutRealmClientReturnsClientError(t *testing.T) {
	mockClient := NewRealmClient(nil)
	client := &identityv1.Client{}
	result, err := mockClient.PutRealmClient(context.Background(), RealmForClientPut, ClientId, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPutRealmClientReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	client := &identityv1.Client{}
	result, err := mockClient.PutRealmClient(context.Background(), RealmForClientPut, ClientId, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPutRealmClientReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	client := &identityv1.Client{}
	result, err := mockClient.PutRealmClient(context.Background(), RealmForPut, ClientId, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPutRealmClientReturnsErrorForEmptyGet(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	client := &identityv1.Client{}
	result, err := mockClient.PutRealmClient(context.Background(), RealmForEmpty, ClientId, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPostRealmClientReturnsClientError(t *testing.T) {
	mockClient := NewRealmClient(nil)
	client := &identityv1.Client{}
	result, err := mockClient.PostRealmClient(context.Background(), RealmForClient, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPostRealmClientReturnsError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	client := &identityv1.Client{}
	result, err := mockClient.PostRealmClient(context.Background(), RealmForClient, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestPostRealmClientReturnsStatusCodeError(t *testing.T) {
	mockClient := NewRealmClientMock(t)
	client := &identityv1.Client{}
	result, err := mockClient.PostRealmClient(context.Background(), Realm, client)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func NewKeycloakClientMock(t *testing.T) *mocks.MockKeycloakClient {
	var mockKeycloakClient = mocks.NewMockKeycloakClient(t)
	return mockKeycloakClient
}

func ConfigureKeycloakClientMock(mockedClient *mocks.MockKeycloakClient) {
	var mockedBody, _ = io.ReadAll(io.NopCloser(strings.NewReader(fmt.Sprintf(`{"realm":"%s"}`, Realm))))

	// The parameter "reqEditors ...RequestEditorFn" is not used in the implementation and therefore omitted
	// in the mock configuration.

	mockedClient.EXPECT().DeleteRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm
		})).
		Return(nil, fmt.Errorf("error deleting realm")).Maybe()

	mockedClient.EXPECT().DeleteRealmClientsIdWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm
		}),
		RealmForClientDelete,
	).
		Return(nil, fmt.Errorf("error deleting realm client")).Maybe()

	mockedClient.EXPECT().GetRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForClient
		})).
		Return(nil, fmt.Errorf("error getting realm")).Maybe()

	mockedClient.EXPECT().GetRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm
		})).
		Return(mockGetRealmResponse(Realm, mockedBody), nil).Maybe()

	mockedClient.EXPECT().GetRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForEmpty
		})).
		Return(mockGetRealmResponseEmpty(mockedBody), nil).Maybe()

	mockedClient.EXPECT().GetRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForPut
		})).
		Return(mockGetRealmResponseOk(RealmForPut, mockedBody), nil).Maybe()

	mockedClient.EXPECT().GetRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForClientPut
		})).
		Return(mockGetRealmResponseOk(RealmForClientPut, mockedBody), nil).Maybe()

	mockedClient.EXPECT().PutRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForClientPut
		}),
		mock.AnythingOfType("api.RealmRepresentation")).
		Return(nil, fmt.Errorf("error updating realm")).Maybe()

	mockedClient.EXPECT().PutRealmWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForPut
		}),
		mock.AnythingOfType("api.RealmRepresentation")).
		Return(mockPutRealmResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PostWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s api.RealmRepresentation) bool {
			return *s.Realm == RealmForClient
		})).
		Return(nil, fmt.Errorf("error creating realm")).Maybe()

	mockedClient.EXPECT().PostWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s api.RealmRepresentation) bool {
			return *s.Realm == Realm
		})).
		Return(mockPostResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().GetRealmClientsWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForClient
		}),
		mock.AnythingOfType("*api.GetRealmClientsParams")).
		Return(nil, fmt.Errorf("error getting realm clients")).Maybe()

	mockedClient.EXPECT().GetRealmClientsWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm
		}),
		mock.AnythingOfType("*api.GetRealmClientsParams")).
		Return(mockGetRealmClientsWithResponse(mockedBody, http.StatusBadRequest, false), nil).Maybe()

	mockedClient.EXPECT().GetRealmClientsWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForEmpty
		}),
		mock.AnythingOfType("*api.GetRealmClientsParams")).
		Return(mockGetRealmClientsWithResponse(mockedBody, http.StatusOK, true), nil).Maybe()

	mockedClient.EXPECT().GetRealmClientsWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForPut
		}),
		mock.AnythingOfType("*api.GetRealmClientsParams")).
		Return(mockGetRealmClientsWithResponse(mockedBody, http.StatusOK, false), nil).Maybe()

	mockedClient.EXPECT().GetRealmClientsWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForClientPut
		}),
		mock.AnythingOfType("*api.GetRealmClientsParams")).
		Return(mockGetRealmClientsWithResponse(mockedBody, http.StatusOK, false), nil).Maybe()

	mockedClient.EXPECT().PutRealmClientsIdWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForClientPut
		}),
		ClientId,
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(nil, fmt.Errorf("error updating realm clients")).Maybe()

	mockedClient.EXPECT().PutRealmClientsIdWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForPut
		}),
		ClientId,
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(mockPutRealmClientsIdResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PutRealmClientsIdWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForEmpty
		}),
		ClientId,
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(mockPutRealmClientsIdResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PostRealmClientsWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == RealmForClient
		}),
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(nil, fmt.Errorf("error creating realm clients")).Maybe()

	mockedClient.EXPECT().PostRealmClientsWithResponse(
		mock.AnythingOfType("context.backgroundCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm
		}),
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(mockPostRealmClientsResponse(mockedBody), nil).Maybe()

}

func NewRealmClientMock(t *testing.T) RealmClient {
	var mockedKeycloakClient = NewKeycloakClientMock(t)
	ConfigureKeycloakClientMock(mockedKeycloakClient)
	realmClient := NewRealmClient(mockedKeycloakClient)
	return realmClient
}

func mockGetRealmResponse(realm string, body []byte) *api.GetRealmResponse {
	return &api.GetRealmResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusBadRequest}),
		JSON2XX:      ptr.To(api.RealmRepresentation{Realm: ptr.To(realm), Enabled: ptr.To(true)}),
	}
}

func mockGetRealmResponseOk(realm string, body []byte) *api.GetRealmResponse {
	return &api.GetRealmResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
		JSON2XX:      ptr.To(api.RealmRepresentation{Realm: ptr.To(realm), Enabled: ptr.To(true)}),
	}
}

func mockGetRealmResponseEmpty(body []byte) *api.GetRealmResponse {
	return &api.GetRealmResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
		JSON2XX:      nil,
	}
}

func mockPutRealmResponse(body []byte) *api.PutRealmResponse {
	return &api.PutRealmResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusBadRequest}),
	}
}

func mockPostResponse(body []byte) *api.PostResponse {
	return &api.PostResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusBadRequest}),
	}
}

func mockGetRealmClientsWithResponse(body []byte, statusCode int, jsonResponseError bool) *api.GetRealmClientsResponse {
	var protocolMapper = api.ProtocolMapperRepresentation{
		Name:           ptr.To("Client ID"),
		Protocol:       ptr.To("openid-connect"),
		ProtocolMapper: ptr.To("oidc-usersessionmodel-note-mapper"),
		Config: &map[string]interface{}{
			"user.session.note":    "clientId",
			"id.token.claim":       "true",
			"access.token.claim":   "true",
			"userinfo.token.claim": "true",
			"claim.name":           "clientId",
			"jsonType.label":       "String",
		},
	}

	var clientRepresentation = api.ClientRepresentation{
		ClientId:               ptr.To(ClientId),
		Name:                   ptr.To(ClientId),
		Enabled:                ptr.To(true),
		FullScopeAllowed:       ptr.To(false),
		ServiceAccountsEnabled: ptr.To(true),
		StandardFlowEnabled:    ptr.To(false),
		Secret:                 ptr.To(ClientSecret),
		ProtocolMappers:        &[]api.ProtocolMapperRepresentation{protocolMapper},
	}

	var json2xx *[]api.ClientRepresentation

	if jsonResponseError {
		json2xx = &[]api.ClientRepresentation{}
	} else {
		json2xx = &[]api.ClientRepresentation{clientRepresentation}
	}
	return &api.GetRealmClientsResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: statusCode}),
		JSON2XX:      json2xx,
	}
}

func mockPutRealmClientsIdResponse(body []byte) *api.PutRealmClientsIdResponse {
	return &api.PutRealmClientsIdResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusBadRequest}),
	}
}

func mockPostRealmClientsResponse(body []byte) *api.PostRealmClientsResponse {
	return &api.PostRealmClientsResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusBadRequest}),
	}
}
