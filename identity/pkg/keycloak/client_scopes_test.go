// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/pkg/keycloak/protocolmappers"
	"github.com/telekom/controlplane/identity/test/mocks/keycloakclient"
)

const testRealm = "test-realm"

// scopeResp builds a GetRealmClientScopesResponse containing the given scopes.
func scopeResp(scopes []api.ClientScopeRepresentation) *api.GetRealmClientScopesResponse {
	return &api.GetRealmClientScopesResponse{
		HTTPResponse: httpResp(200),
		JSON2XX:      &scopes,
	}
}

// emptyScopeResp returns a response with no scopes.
func emptyScopeResp() *api.GetRealmClientScopesResponse {
	return scopeResp([]api.ClientScopeRepresentation{})
}

// defaultScopesResp builds a GetRealmDefaultDefaultClientScopesResponse.
func defaultScopesResp(scopes []api.ClientScopeRepresentation) *api.GetRealmDefaultDefaultClientScopesResponse {
	return &api.GetRealmDefaultDefaultClientScopesResponse{
		HTTPResponse: httpResp(200),
		JSON2XX:      &scopes,
	}
}

// emptyDefaultScopesResp returns a response with no default scopes.
func emptyDefaultScopesResp() *api.GetRealmDefaultDefaultClientScopesResponse {
	return defaultScopesResp([]api.ClientScopeRepresentation{})
}

// managedScope builds a ClientScopeRepresentation mimicking the managed scope
// in Keycloak, with the given mappers and a Keycloak-assigned scope ID.
func managedScope(scopeID string, mappers []api.ProtocolMapperRepresentation) api.ClientScopeRepresentation {
	return api.ClientScopeRepresentation{
		Id:              ptr.To(scopeID),
		Name:            ptr.To(keycloak.ManagedClientScopeName),
		Protocol:        ptr.To("openid-connect"),
		ProtocolMappers: &mappers,
	}
}

// mapperWithID returns a hardcoded-claim mapper with a Keycloak-assigned ID,
// simulating what Keycloak returns for an existing mapper.
func mapperWithID(id, claimName, claimValue string) api.ProtocolMapperRepresentation {
	m := protocolmappers.NewHardcodedClaimMapper(claimName, claimValue)
	m.Id = ptr.To(id)
	return m
}

var _ = Describe("ConfigureClientScopes", func() {
	var (
		ctx        context.Context
		mockClient *keycloakclient.MockKeycloakClient
		svc        keycloak.KeycloakService
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = keycloakclient.NewMockKeycloakClient(GinkgoT())
		svc = keycloak.NewKeycloakService(mockClient)
	})

	When("claims is empty and no managed scope exists", func() {
		It("should be a no-op", func() {
			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyScopeResp(), nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("claims is empty and managed scope exists", func() {
		It("should delete the scope", func() {
			scopeID := "scope-123"
			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(scopeResp([]api.ClientScopeRepresentation{
					managedScope(scopeID, nil),
				}), nil)

			mockClient.EXPECT().
				DeleteRealmDefaultDefaultClientScopesClientScopeIdWithResponse(mock.Anything, testRealm, scopeID).
				Return(&api.DeleteRealmDefaultDefaultClientScopesClientScopeIdResponse{
					HTTPResponse: httpResp(204),
				}, nil)

			mockClient.EXPECT().
				DeleteRealmClientScopesIdWithResponse(mock.Anything, testRealm, scopeID).
				Return(&api.DeleteRealmClientScopesIdResponse{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, []identityv1.ClaimConfig{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("claims provided and no managed scope exists", func() {
		It("should create the scope and assign as realm default", func() {
			claims := []identityv1.ClaimConfig{
				{Name: "env", Value: "prod"},
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyScopeResp(), nil)

			mockClient.EXPECT().
				PostRealmClientScopesWithResponse(mock.Anything, testRealm, mock.Anything).
				Return(&api.PostRealmClientScopesResponse{
					HTTPResponse: httpRespWithLocation(201, "/realms/test-realm/client-scopes/new-id"),
				}, nil)

			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyDefaultScopesResp(), nil)

			mockClient.EXPECT().
				PutRealmDefaultDefaultClientScopesClientScopeIdWithResponse(mock.Anything, testRealm, "new-id").
				Return(&api.PutRealmDefaultDefaultClientScopesClientScopeIdResponse{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("claims match existing mappers", func() {
		It("should skip update (no-op)", func() {
			scopeID := "scope-123"
			claims := []identityv1.ClaimConfig{
				{Name: "env", Value: "prod"},
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(scopeResp([]api.ClientScopeRepresentation{
					managedScope(scopeID, []api.ProtocolMapperRepresentation{
						mapperWithID("m1", "env", "prod"),
					}),
				}), nil)

			// Scope is already a realm default — no PUT needed.
			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(defaultScopesResp([]api.ClientScopeRepresentation{
					{Id: ptr.To(scopeID)},
				}), nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("a new mapper is added", func() {
		It("should create only the new mapper", func() {
			scopeID := "scope-123"
			claims := []identityv1.ClaimConfig{
				{Name: "env", Value: "prod"},
				{Name: "team", Value: "hyperion"},
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(scopeResp([]api.ClientScopeRepresentation{
					managedScope(scopeID, []api.ProtocolMapperRepresentation{
						mapperWithID("m1", "env", "prod"),
					}),
				}), nil)

			// Scope is already a realm default — no PUT needed.
			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(defaultScopesResp([]api.ClientScopeRepresentation{
					{Id: ptr.To(scopeID)},
				}), nil)

			// Only the new mapper should be created.
			mockClient.EXPECT().
				PostRealmClientScopesIdProtocolMappersModelsWithResponse(mock.Anything, testRealm, scopeID, mock.MatchedBy(func(m api.ProtocolMapperRepresentation) bool {
					return m.Name != nil && *m.Name == "controlplane-claim-team"
				})).
				Return(&api.PostRealmClientScopesIdProtocolMappersModelsResponse{
					HTTPResponse: httpResp(201),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("a mapper is removed", func() {
		It("should delete only the removed mapper", func() {
			scopeID := "scope-123"
			claims := []identityv1.ClaimConfig{
				{Name: "env", Value: "prod"},
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(scopeResp([]api.ClientScopeRepresentation{
					managedScope(scopeID, []api.ProtocolMapperRepresentation{
						mapperWithID("m1", "env", "prod"),
						mapperWithID("m2", "team", "hyperion"),
					}),
				}), nil)

			// Scope is already a realm default — no PUT needed.
			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(defaultScopesResp([]api.ClientScopeRepresentation{
					{Id: ptr.To(scopeID)},
				}), nil)

			// Only the removed mapper should be deleted.
			mockClient.EXPECT().
				DeleteRealmClientScopesId1ProtocolMappersModelsId2WithResponse(mock.Anything, testRealm, scopeID, "m2").
				Return(&api.DeleteRealmClientScopesId1ProtocolMappersModelsId2Response{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("a mapper config changed", func() {
		It("should update only the changed mapper", func() {
			scopeID := "scope-123"
			claims := []identityv1.ClaimConfig{
				{Name: "env", Value: "staging"}, // changed from "prod"
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(scopeResp([]api.ClientScopeRepresentation{
					managedScope(scopeID, []api.ProtocolMapperRepresentation{
						mapperWithID("m1", "env", "prod"),
					}),
				}), nil)

			// Scope is already a realm default — no PUT needed.
			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(defaultScopesResp([]api.ClientScopeRepresentation{
					{Id: ptr.To(scopeID)},
				}), nil)

			mockClient.EXPECT().
				PutRealmClientScopesId1ProtocolMappersModelsId2WithResponse(mock.Anything, testRealm, scopeID, "m1", mock.MatchedBy(func(m api.ProtocolMapperRepresentation) bool {
					return m.Id != nil && *m.Id == "m1" &&
						m.Config != nil && (*m.Config)["claim.value"] == "staging"
				})).
				Return(&api.PutRealmClientScopesId1ProtocolMappersModelsId2Response{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("mixed changes: add, remove, update", func() {
		It("should apply all changes correctly", func() {
			scopeID := "scope-123"
			claims := []identityv1.ClaimConfig{
				{Name: "env", Value: "staging"},    // updated (was "prod")
				{Name: "region", Value: "eu-west"}, // new
				// "team" removed
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(scopeResp([]api.ClientScopeRepresentation{
					managedScope(scopeID, []api.ProtocolMapperRepresentation{
						mapperWithID("m1", "env", "prod"),
						mapperWithID("m2", "team", "hyperion"),
					}),
				}), nil)

			// Scope is already a realm default — no PUT needed.
			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(defaultScopesResp([]api.ClientScopeRepresentation{
					{Id: ptr.To(scopeID)},
				}), nil)

			// Delete removed mapper.
			mockClient.EXPECT().
				DeleteRealmClientScopesId1ProtocolMappersModelsId2WithResponse(mock.Anything, testRealm, scopeID, "m2").
				Return(&api.DeleteRealmClientScopesId1ProtocolMappersModelsId2Response{
					HTTPResponse: httpResp(204),
				}, nil)

			// Create new mapper.
			mockClient.EXPECT().
				PostRealmClientScopesIdProtocolMappersModelsWithResponse(mock.Anything, testRealm, scopeID, mock.MatchedBy(func(m api.ProtocolMapperRepresentation) bool {
					return m.Name != nil && *m.Name == "controlplane-claim-region"
				})).
				Return(&api.PostRealmClientScopesIdProtocolMappersModelsResponse{
					HTTPResponse: httpResp(201),
				}, nil)

			// Update changed mapper.
			mockClient.EXPECT().
				PutRealmClientScopesId1ProtocolMappersModelsId2WithResponse(mock.Anything, testRealm, scopeID, "m1", mock.MatchedBy(func(m api.ProtocolMapperRepresentation) bool {
					return m.Config != nil && (*m.Config)["claim.value"] == "staging"
				})).
				Return(&api.PutRealmClientScopesId1ProtocolMappersModelsId2Response{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("claims include a SessionNote type", func() {
		It("should create the scope with a session-note mapper", func() {
			claims := []identityv1.ClaimConfig{
				{Name: "clientId", Type: identityv1.ClaimTypeSessionNote},
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyScopeResp(), nil)

			mockClient.EXPECT().
				PostRealmClientScopesWithResponse(mock.Anything, testRealm, mock.MatchedBy(func(scope api.ClientScopeRepresentation) bool {
					if scope.ProtocolMappers == nil || len(*scope.ProtocolMappers) != 1 {
						return false
					}
					m := (*scope.ProtocolMappers)[0]
					return m.ProtocolMapper != nil && *m.ProtocolMapper == "oidc-usersessionmodel-note-mapper" &&
						m.Config != nil && (*m.Config)["user.session.note"] == "clientId"
				})).
				Return(&api.PostRealmClientScopesResponse{
					HTTPResponse: httpRespWithLocation(201, "/realms/test-realm/client-scopes/new-id"),
				}, nil)

			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyDefaultScopesResp(), nil)

			mockClient.EXPECT().
				PutRealmDefaultDefaultClientScopesClientScopeIdWithResponse(mock.Anything, testRealm, "new-id").
				Return(&api.PutRealmDefaultDefaultClientScopesClientScopeIdResponse{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("SessionNote claim omits Value", func() {
		It("should default the session note key to the claim Name", func() {
			claims := []identityv1.ClaimConfig{
				{Name: "clientId", Type: identityv1.ClaimTypeSessionNote, Value: ""},
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyScopeResp(), nil)

			mockClient.EXPECT().
				PostRealmClientScopesWithResponse(mock.Anything, testRealm, mock.MatchedBy(func(scope api.ClientScopeRepresentation) bool {
					m := (*scope.ProtocolMappers)[0]
					return (*m.Config)["user.session.note"] == "clientId"
				})).
				Return(&api.PostRealmClientScopesResponse{
					HTTPResponse: httpRespWithLocation(201, "/realms/test-realm/client-scopes/new-id"),
				}, nil)

			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyDefaultScopesResp(), nil)

			mockClient.EXPECT().
				PutRealmDefaultDefaultClientScopesClientScopeIdWithResponse(mock.Anything, testRealm, "new-id").
				Return(&api.PutRealmDefaultDefaultClientScopesClientScopeIdResponse{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("mixed hardcoded and session-note claims", func() {
		It("should create the scope with both mapper types", func() {
			claims := []identityv1.ClaimConfig{
				{Name: "env", Value: "prod", Type: identityv1.ClaimTypeHardcodedClaim},
				{Name: "clientId", Type: identityv1.ClaimTypeSessionNote},
			}

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyScopeResp(), nil)

			mockClient.EXPECT().
				PostRealmClientScopesWithResponse(mock.Anything, testRealm, mock.MatchedBy(func(scope api.ClientScopeRepresentation) bool {
					if scope.ProtocolMappers == nil || len(*scope.ProtocolMappers) != 2 {
						return false
					}
					mappers := *scope.ProtocolMappers
					hasHardcoded := false
					hasSessionNote := false
					for _, m := range mappers {
						if m.ProtocolMapper != nil {
							switch *m.ProtocolMapper {
							case "oidc-hardcoded-claim-mapper":
								hasHardcoded = true
							case "oidc-usersessionmodel-note-mapper":
								hasSessionNote = true
							}
						}
					}
					return hasHardcoded && hasSessionNote
				})).
				Return(&api.PostRealmClientScopesResponse{
					HTTPResponse: httpRespWithLocation(201, "/realms/test-realm/client-scopes/new-id"),
				}, nil)

			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(emptyDefaultScopesResp(), nil)

			mockClient.EXPECT().
				PutRealmDefaultDefaultClientScopesClientScopeIdWithResponse(mock.Anything, testRealm, "new-id").
				Return(&api.PutRealmDefaultDefaultClientScopesClientScopeIdResponse{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("claim type changes from hardcoded to session-note", func() {
		It("should detect the change and update the mapper", func() {
			scopeID := "scope-123"
			claims := []identityv1.ClaimConfig{
				{Name: "clientId", Type: identityv1.ClaimTypeSessionNote},
			}

			// Existing scope has a hardcoded mapper for "clientId".
			existingMapper := mapperWithID("m1", "clientId", "static-value")

			mockClient.EXPECT().
				GetRealmClientScopesWithResponse(mock.Anything, testRealm).
				Return(scopeResp([]api.ClientScopeRepresentation{
					managedScope(scopeID, []api.ProtocolMapperRepresentation{existingMapper}),
				}), nil)

			// Scope is already a realm default — no PUT needed.
			mockClient.EXPECT().
				GetRealmDefaultDefaultClientScopesWithResponse(mock.Anything, testRealm).
				Return(defaultScopesResp([]api.ClientScopeRepresentation{
					{Id: ptr.To(scopeID)},
				}), nil)

			// The mapper type changed, so it should be updated.
			mockClient.EXPECT().
				PutRealmClientScopesId1ProtocolMappersModelsId2WithResponse(mock.Anything, testRealm, scopeID, "m1", mock.MatchedBy(func(m api.ProtocolMapperRepresentation) bool {
					return m.ProtocolMapper != nil && *m.ProtocolMapper == "oidc-usersessionmodel-note-mapper"
				})).
				Return(&api.PutRealmClientScopesId1ProtocolMappersModelsId2Response{
					HTTPResponse: httpResp(204),
				}, nil)

			err := svc.ConfigureClientScopes(ctx, testRealm, claims)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
