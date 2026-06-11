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
	"github.com/telekom/controlplane/identity/test/mocks/keycloakclient"
)

const (
	Realm              = "test-realm"
	RealmForClient     = "realm-test-client"
	ClientId           = "test-client"
	ClientSecret       = "test-secret"
	protocolMapperTrue = "true"
)

func NewKeycloakClientMock(testing ginkgo.FullGinkgoTInterface) *keycloakclient.MockKeycloakClient {
	// Construct manually instead of using keycloakclient.NewMockKeycloakClient(testing),
	// because the generated constructor calls t.Cleanup() which maps to
	// DeferCleanup() in Ginkgo. When called from BeforeSuite, this creates a
	// nested DeferCleanup-inside-DeferCleanup which Ginkgo forbids.
	mockKeycloakClient := &keycloakclient.MockKeycloakClient{}
	mockKeycloakClient.Test(testing)
	return mockKeycloakClient
}

func ConfigureKeycloakClientMock(mockedClient *keycloakclient.MockKeycloakClient) {
	mockedBody, err := io.ReadAll(io.NopCloser(strings.NewReader(fmt.Sprintf(`{"realm":%q}`, Realm))))
	if err != nil {
		panic(fmt.Sprintf("read mocked realm body: %v", err))
	}

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

	// Client scope operations used by ConfigureClientScopes.
	mockedClient.EXPECT().GetRealmClientScopesWithResponse(
		mock.Anything,
		realmMatcher).
		Return(&api.GetRealmClientScopesResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
			JSON2XX:      &[]api.ClientScopeRepresentation{},
		}, nil).Maybe()

	mockedClient.EXPECT().PostRealmClientScopesWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("api.ClientScopeRepresentation")).
		Return(&api.PostRealmClientScopesResponse{
			HTTPResponse: &http.Response{
				StatusCode: http.StatusCreated,
				Header:     http.Header{"Location": {fmt.Sprintf("/realms/%s/client-scopes/mock-scope-id", Realm)}},
			},
		}, nil).Maybe()

	mockedClient.EXPECT().GetRealmDefaultDefaultClientScopesWithResponse(
		mock.Anything,
		realmMatcher).
		Return(&api.GetRealmDefaultDefaultClientScopesResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON2XX:      ptr.To([]api.ClientScopeRepresentation{}),
		}, nil).Maybe()

	mockedClient.EXPECT().PutRealmDefaultDefaultClientScopesClientScopeIdWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string")).
		Return(&api.PutRealmDefaultDefaultClientScopesClientScopeIdResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
		}, nil).Maybe()

	mockedClient.EXPECT().DeleteRealmDefaultDefaultClientScopesClientScopeIdWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string")).
		Return(&api.DeleteRealmDefaultDefaultClientScopesClientScopeIdResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
		}, nil).Maybe()

	mockedClient.EXPECT().DeleteRealmClientScopesIdWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string")).
		Return(&api.DeleteRealmClientScopesIdResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
		}, nil).Maybe()

	mockedClient.EXPECT().PostRealmClientScopesIdProtocolMappersModelsWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string"),
		mock.AnythingOfType("api.ProtocolMapperRepresentation")).
		Return(&api.PostRealmClientScopesIdProtocolMappersModelsResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusCreated}),
		}, nil).Maybe()

	mockedClient.EXPECT().PutRealmClientScopesId1ProtocolMappersModelsId2WithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("api.ProtocolMapperRepresentation")).
		Return(&api.PutRealmClientScopesId1ProtocolMappersModelsId2Response{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
		}, nil).Maybe()

	mockedClient.EXPECT().DeleteRealmClientScopesId1ProtocolMappersModelsId2WithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).
		Return(&api.DeleteRealmClientScopesId1ProtocolMappersModelsId2Response{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
		}, nil).Maybe()

	// Client-policy profile and policy operations used by
	// ConfigureSecretRotationPolicy / DeleteSecretRotationPolicy.
	mockedClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("*api.GetRealmClientPoliciesProfilesParams")).
		Return(&api.GetRealmClientPoliciesProfilesResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
			JSON2XX:      &api.ClientProfilesRepresentation{},
		}, nil).Maybe()

	mockedClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("api.ClientProfilesRepresentation")).
		Return(&api.PutRealmClientPoliciesProfilesResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
		}, nil).Maybe()

	mockedClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("*api.GetRealmClientPoliciesPoliciesParams")).
		Return(&api.GetRealmClientPoliciesPoliciesResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
			JSON2XX:      &api.ClientPoliciesRepresentation{},
		}, nil).Maybe()

	mockedClient.EXPECT().PutRealmClientPoliciesPoliciesWithResponse(
		mock.Anything,
		realmMatcher,
		mock.AnythingOfType("api.ClientPoliciesRepresentation")).
		Return(&api.PutRealmClientPoliciesPoliciesResponse{
			HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusNoContent}),
		}, nil).Maybe()
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
	protocolMapper := api.ProtocolMapperRepresentation{
		Name:           ptr.To("Client ID"),
		Protocol:       ptr.To("openid-connect"),
		ProtocolMapper: ptr.To("oidc-usersessionmodel-note-mapper"),
		Config: &map[string]interface{}{
			"user.session.note":    "clientId",
			"id.token.claim":       protocolMapperTrue,
			"access.token.claim":   protocolMapperTrue,
			"userinfo.token.claim": protocolMapperTrue,
			"claim.name":           "clientId",
			"jsonType.label":       "String",
		},
	}

	clientRepresentation := api.ClientRepresentation{
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
