// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"

	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/test/mocks"
)

const (
	Realm          = "test-realm"
	RealmForClient = "realm-test-client"
	ClientId       = "test-client"
	ClientSecret   = "test-secret"
)

func NewKeycloakClientMock(testing ginkgo.FullGinkgoTInterface) *mocks.MockKeycloakClient {
	// Construct manually instead of using mocks.NewMockKeycloakClient(testing),
	// because the generated constructor calls t.Cleanup() which maps to
	// DeferCleanup() in Ginkgo. When called from BeforeSuite, this creates a
	// nested DeferCleanup-inside-DeferCleanup which Ginkgo forbids.
	mockKeycloakClient := &mocks.MockKeycloakClient{}
	mockKeycloakClient.Mock.Test(testing)
	return mockKeycloakClient
}

func ConfigureKeycloakClientMock(mockedClient *mocks.MockKeycloakClient) {
	var mockedBody, _ = io.ReadAll(io.NopCloser(strings.NewReader(fmt.Sprintf(`{"realm":"%s"}`, Realm))))

	// The parameter "reqEditors ...RequestEditorFn" is not used in the implementation and therefore omitted
	// in the mock configuration.
	//
	// We use mock.Anything for the context parameter to avoid brittle type
	// assertions — the exact concrete context type may vary depending on
	// how controller-runtime wraps the reconciler context.

	realmMatcher := mock.MatchedBy(func(s string) bool {
		return s == Realm || s == RealmForClient
	})

	mockedClient.EXPECT().GetRealmWithResponse(
		mock.Anything,
		realmMatcher).
		Return(mockGetRealmResponse(Realm, mockedBody), nil).Maybe()

	mockedClient.EXPECT().DeleteRealmWithResponse(
		mock.Anything,
		realmMatcher).
		Return(mockDeleteRealmResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().DeleteRealmClientsIdWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string")).
		Return(mockDeleteRealmClientsIdResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PutRealmWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("api.RealmRepresentation")).
		Return(mockPutRealmResponse(mockedBody), nil).Maybe()

	// PostWithResponse creates a new realm: signature is (ctx, body RealmRepresentation, ...RequestEditorFn)
	mockedClient.EXPECT().PostWithResponse(
		mock.Anything,
		mock.AnythingOfType("api.RealmRepresentation")).
		Return(mockPostResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().GetRealmClientsWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("*api.GetRealmClientsParams")).
		Return(mockGetRealmClientsWithResponse(mockedBody, ClientId, ClientSecret), nil).Maybe()

	mockedClient.EXPECT().GetRealmClientsIdWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string")).
		Return(mockGetRealmClientsIdResponse(ClientId, ClientSecret), nil).Maybe()

	mockedClient.EXPECT().PutRealmClientsIdWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string"),
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(mockPutRealmClientsIdResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PostRealmClientsWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(mockPostRealmClientsResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PostRealmClientsIdClientSecretWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string")).
		Return(mockPostRealmClientsIdClientSecretResponse(), nil).Maybe()

	// GetRealmClientsIdClientSecretRotatedWithResponse is called by the client
	// handler to check for an active graceful rotation. Return 404 (no rotation).
	mockedClient.EXPECT().GetRealmClientsIdClientSecretRotatedWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string")).
		Return(mockGetRealmClientsIdClientSecretRotatedResponse(), nil).Maybe()

}

func mockGetRealmResponse(realm string, body []byte) *api.GetRealmResponse {
	return &api.GetRealmResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
		JSON2XX:      ptr.To(api.RealmRepresentation{Realm: ptr.To(realm), Enabled: ptr.To(true)}),
	}
}

func mockDeleteRealmResponse(body []byte) *api.DeleteRealmResponse {
	return &api.DeleteRealmResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
	}
}

func mockDeleteRealmClientsIdResponse(body []byte) *api.DeleteRealmClientsIdResponse {
	return &api.DeleteRealmClientsIdResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
	}
}

func mockPutRealmResponse(body []byte) *api.PutRealmResponse {
	return &api.PutRealmResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
	}
}

func mockPostResponse(body []byte) *api.PostResponse {
	return &api.PostResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusCreated}),
	}
}

func mockGetRealmClientsWithResponse(body []byte, clientId, clientSecret string) *api.GetRealmClientsResponse {
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
		Id:                     ptr.To("mock-client-uuid"),
		ClientId:               ptr.To(clientId),
		Name:                   ptr.To(clientId),
		Enabled:                ptr.To(true),
		FullScopeAllowed:       ptr.To(false),
		ServiceAccountsEnabled: ptr.To(true),
		StandardFlowEnabled:    ptr.To(false),
		Secret:                 &clientSecret,
		ProtocolMappers:        &[]api.ProtocolMapperRepresentation{protocolMapper},
	}

	return &api.GetRealmClientsResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
		JSON2XX:      &[]api.ClientRepresentation{clientRepresentation},
	}
}

func mockPutRealmClientsIdResponse(body []byte) *api.PutRealmClientsIdResponse {
	return &api.PutRealmClientsIdResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
	}
}

func mockPostRealmClientsResponse(body []byte) *api.PostRealmClientsResponse {
	return &api.PostRealmClientsResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusCreated}),
	}
}

func mockPostRealmClientsIdClientSecretResponse() *api.PostRealmClientsIdClientSecretResponse {
	return &api.PostRealmClientsIdClientSecretResponse{
		Body:         []byte(`{}`),
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
		JSON2XX:      &api.CredentialRepresentation{},
	}
}

func mockGetRealmClientsIdResponse(clientId, clientSecret string) *api.GetRealmClientsIdResponse {
	return &api.GetRealmClientsIdResponse{
		Body:         []byte(`{}`),
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
		JSON2XX: &api.ClientRepresentation{
			Id:                     ptr.To("mock-client-uuid"),
			ClientId:               ptr.To(clientId),
			Name:                   ptr.To(clientId),
			Enabled:                ptr.To(true),
			FullScopeAllowed:       ptr.To(false),
			ServiceAccountsEnabled: ptr.To(true),
			StandardFlowEnabled:    ptr.To(false),
			Secret:                 ptr.To(clientSecret),
		},
	}
}

func mockGetRealmClientsIdClientSecretRotatedResponse() *api.GetRealmClientsIdClientSecretRotatedResponse {
	return &api.GetRealmClientsIdClientSecretRotatedResponse{
		Body:         []byte(`{}`),
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNotFound}),
	}
}
