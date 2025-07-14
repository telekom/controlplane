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
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/mocks"
)

const (
	Realm          = "test-realm"
	RealmForClient = "realm-test-client"
	ClientId       = "test-client"
	ClientSecret   = "test-secret"
)

// NewKeycloakClientMock creates a new KeycloakClient mock without using t.Cleanup to avoid DeferCleanup conflicts with Ginkgo
func NewKeycloakClientMock(testing ginkgo.FullGinkgoTInterface) *mocks.MockKeycloakClient {
	// Create mock directly without using t.Cleanup in the mock constructor
	mock := &mocks.MockKeycloakClient{}
	mock.Test(testing)

	// Register manual cleanup using Ginkgo's DeferCleanup
	ginkgo.DeferCleanup(func() {
		mock.AssertExpectations(testing)
	})

	return mock
}

func ConfigureKeycloakClientMock(mockedClient *mocks.MockKeycloakClient) {
	var mockedBody, _ = io.ReadAll(io.NopCloser(strings.NewReader(fmt.Sprintf(`{"realm":"%s"}`, Realm))))

	// The parameter "reqEditors ...RequestEditorFn" is not used in the implementation and therefore omitted
	// in the mock configuration.

	mockedClient.EXPECT().GetRealmWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		})).
		Return(mockGetRealmResponse(Realm, mockedBody), nil).Maybe()

	mockedClient.EXPECT().DeleteRealmWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		})).
		Return(mockDeleteRealmResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().DeleteRealmClientsIdWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		}), ClientId).
		Return(mockDeleteRealmClientsIdResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PutRealmWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		}),
		mock.AnythingOfType("api.RealmRepresentation")).
		Return(mockPutRealmResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PostWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		}),
		mock.AnythingOfType("api.PostResponse"),
		mock.Anything).
		Return(mockPostResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().GetRealmClientsWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		}),
		mock.AnythingOfType("*api.GetRealmClientsParams")).
		Return(mockGetRealmClientsWithResponse(mockedBody, ClientId, ClientSecret), nil).Maybe()

	mockedClient.EXPECT().PutRealmClientsIdWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		}),
		ClientId,
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(mockPutRealmClientsIdResponse(mockedBody), nil).Maybe()

	mockedClient.EXPECT().PostRealmClientsWithResponse(
		mock.AnythingOfType("*context.valueCtx"),
		mock.MatchedBy(func(s string) bool {
			return s == Realm || s == RealmForClient
		}),
		mock.AnythingOfType("api.ClientRepresentation")).
		Return(mockPostRealmClientsResponse(mockedBody), nil).Maybe()

}

func NewRealmClientMock(testing ginkgo.FullGinkgoTInterface) keycloak.RealmClient {
	var mockedKeycloakClient = NewKeycloakClientMock(testing)
	ConfigureKeycloakClientMock(mockedKeycloakClient)
	realmClient := keycloak.NewRealmClient(mockedKeycloakClient)
	return realmClient
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
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
	}
}

func mockDeleteRealmClientsIdResponse(body []byte) *api.DeleteRealmClientsIdResponse {
	return &api.DeleteRealmClientsIdResponse{
		Body:         body,
		HTTPResponse: ptr.To(http.Response{StatusCode: http.StatusOK}),
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
