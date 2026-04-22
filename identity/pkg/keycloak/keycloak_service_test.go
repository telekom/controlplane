// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/pkg/api"
	"github.com/telekom/controlplane/identity/pkg/keycloak"
	"github.com/telekom/controlplane/identity/test/mocks/keycloakclient"
)

// helper to build a minimal http.Response with a given status code.
func httpResp(code int) *http.Response {
	return &http.Response{StatusCode: code}
}

// helper to build a minimal http.Response with a Location header.
func httpRespWithLocation(code int, location string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Location": {location}},
	}
}

func newIdentityClient(clientId, secret string) *identityv1.Client {
	return &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: "test-client", Namespace: "default"},
		Spec:       identityv1.ClientSpec{ClientId: clientId, ClientSecret: secret},
	}
}

func newIdentityClientWithUID(clientId, secret, uid string) *identityv1.Client {
	c := newIdentityClient(clientId, secret)
	c.Status.ClientUid = uid
	return c
}

func newRealm(name string) *identityv1.Realm {
	return &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
}

func newMockClient() *keycloakclient.MockKeycloakClient {
	m := &keycloakclient.MockKeycloakClient{}
	m.Mock.Test(GinkgoT())
	return m
}

var _ = Describe("KeycloakService", func() {

	var (
		mockClient *keycloakclient.MockKeycloakClient
		svc        keycloak.KeycloakService
		ctx        context.Context
	)

	BeforeEach(func() {
		mockClient = newMockClient()
		svc = keycloak.NewKeycloakService(mockClient)
		ctx = context.Background()
	})

	// ── CreateOrReplaceClient ──────────────────────────────────────────

	Describe("CreateOrReplaceClient", func() {

		Context("when client does not exist (create path)", func() {

			BeforeEach(func() {
				// getClient returns empty list (no UID on client, search by clientId)
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{},
					}, nil)
			})

			It("should create the client and extract UID from Location header", func() {
				client := newIdentityClient("my-app", "my-secret")
				mockClient.EXPECT().PostRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PostRealmClientsResponse{
						HTTPResponse: httpRespWithLocation(201, "https://kc/admin/realms/realm1/clients/new-uid"),
					}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(client.Status.ClientUid).To(Equal("new-uid"))
			})

			It("should return error when POST fails with network error", func() {
				client := newIdentityClient("my-app", "secret")
				mockClient.EXPECT().PostRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(nil, fmt.Errorf("connection refused"))

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error creating client"))
			})

			It("should return error when POST returns non-201 status", func() {
				client := newIdentityClient("my-app", "secret")
				mockClient.EXPECT().PostRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PostRealmClientsResponse{HTTPResponse: httpResp(409)}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("creating client"))
			})

			It("should return error when Location header is missing", func() {
				client := newIdentityClient("my-app", "secret")
				mockClient.EXPECT().PostRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PostRealmClientsResponse{
						HTTPResponse: &http.Response{StatusCode: 201, Header: http.Header{}},
					}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to extract ID"))
			})
		})

		Context("when client exists (update path)", func() {

			It("should update existing client and set UID in status", func() {
				client := newIdentityClient("my-app", "new-secret")
				existing := api.ClientRepresentation{
					Id:       ptr.To("existing-uid"),
					ClientId: ptr.To("my-app"),
					Secret:   ptr.To("old-secret"),
				}
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{existing},
					}, nil)
				mockClient.EXPECT().PutRealmClientsIdWithResponse(mock.Anything, "realm1", "existing-uid", mock.Anything).
					Return(&api.PutRealmClientsIdResponse{HTTPResponse: httpResp(204)}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(client.Status.ClientUid).To(Equal("existing-uid"))
			})

			It("should return error when PUT fails", func() {
				client := newIdentityClient("my-app", "new-secret")
				existing := api.ClientRepresentation{
					Id:       ptr.To("uid"),
					ClientId: ptr.To("my-app"),
					Secret:   ptr.To("old-secret"),
				}
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{existing},
					}, nil)
				mockClient.EXPECT().PutRealmClientsIdWithResponse(mock.Anything, "realm1", "uid", mock.Anything).
					Return(nil, fmt.Errorf("timeout"))

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error updating client"))
			})

			It("should force secret rotation before update when secret changed and graceful rotation enabled", func() {
				client := newIdentityClient("my-app", "new-secret")
				existing := api.ClientRepresentation{
					Id:       ptr.To("existing-uid"),
					ClientId: ptr.To("my-app"),
					Name:     ptr.To("my-app"),
					Secret:   ptr.To("old-secret"),
				}
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{existing},
					}, nil)
				// forceSecretRotation calls
				mockClient.EXPECT().DeleteRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "existing-uid").
					Return(&api.DeleteRealmClientsIdClientSecretRotatedResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().PostRealmClientsIdClientSecretWithResponse(mock.Anything, "realm1", "existing-uid").
					Return(&api.PostRealmClientsIdClientSecretResponse{HTTPResponse: httpResp(200)}, nil)
				// re-fetch after rotation
				mockClient.EXPECT().GetRealmClientsIdWithResponse(mock.Anything, "realm1", "existing-uid").
					Return(&api.GetRealmClientsIdResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientRepresentation{Id: ptr.To("existing-uid"), ClientId: ptr.To("my-app"), Secret: ptr.To("rotated-random")},
					}, nil)
				// PUT update
				mockClient.EXPECT().PutRealmClientsIdWithResponse(mock.Anything, "realm1", "existing-uid", mock.Anything).
					Return(&api.PutRealmClientsIdResponse{HTTPResponse: httpResp(204)}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, true)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return error when forceSecretRotation POST fails", func() {
				client := newIdentityClient("my-app", "new-secret")
				existing := api.ClientRepresentation{
					Id:       ptr.To("uid"),
					ClientId: ptr.To("my-app"),
					Secret:   ptr.To("old-secret"),
				}
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{existing},
					}, nil)
				mockClient.EXPECT().DeleteRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "uid").
					Return(&api.DeleteRealmClientsIdClientSecretRotatedResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().PostRealmClientsIdClientSecretWithResponse(mock.Anything, "realm1", "uid").
					Return(nil, fmt.Errorf("rotation failed"))

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, true)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to force secret rotation"))
			})

			It("should return error when re-fetch after rotation returns nil JSON", func() {
				client := newIdentityClient("my-app", "new-secret")
				existing := api.ClientRepresentation{
					Id:       ptr.To("uid"),
					ClientId: ptr.To("my-app"),
					Secret:   ptr.To("old-secret"),
				}
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{existing},
					}, nil)
				mockClient.EXPECT().DeleteRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "uid").
					Return(&api.DeleteRealmClientsIdClientSecretRotatedResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().PostRealmClientsIdClientSecretWithResponse(mock.Anything, "realm1", "uid").
					Return(&api.PostRealmClientsIdClientSecretResponse{HTTPResponse: httpResp(200)}, nil)
				mockClient.EXPECT().GetRealmClientsIdWithResponse(mock.Anything, "realm1", "uid").
					Return(&api.GetRealmClientsIdResponse{
						HTTPResponse: httpResp(404),
						JSON2XX:      nil,
					}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, true)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("re-fetching client after rotation"))
			})
		})

		Context("when client lookup by UID succeeds", func() {

			It("should find client by UID and update", func() {
				client := newIdentityClientWithUID("my-app", "new-secret", "uuid-123")
				mockClient.EXPECT().GetRealmClientsIdWithResponse(mock.Anything, "realm1", "uuid-123").
					Return(&api.GetRealmClientsIdResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientRepresentation{Id: ptr.To("uuid-123"), ClientId: ptr.To("my-app"), Secret: ptr.To("old-secret")},
					}, nil)
				mockClient.EXPECT().PutRealmClientsIdWithResponse(mock.Anything, "realm1", "uuid-123", mock.Anything).
					Return(&api.PutRealmClientsIdResponse{HTTPResponse: httpResp(204)}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should fall back to clientId search when UID returns 404", func() {
				client := newIdentityClientWithUID("my-app", "secret", "stale-uid")
				mockClient.EXPECT().GetRealmClientsIdWithResponse(mock.Anything, "realm1", "stale-uid").
					Return(&api.GetRealmClientsIdResponse{HTTPResponse: httpResp(404)}, nil)
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{},
					}, nil)
				mockClient.EXPECT().PostRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PostRealmClientsResponse{
						HTTPResponse: httpRespWithLocation(201, "https://kc/clients/new-uid"),
					}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(client.Status.ClientUid).To(Equal("new-uid"))
			})
		})

		Context("when getClient fails", func() {

			DescribeTable("should propagate error",
				func(networkErr error, expectedSubstring string) {
					client := newIdentityClient("my-app", "secret")
					mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
						Return(nil, networkErr)

					err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedSubstring))
				},
				Entry("network error", fmt.Errorf("network error"), "error checking for existing client"),
				Entry("timeout", fmt.Errorf("timeout"), "error checking for existing client"),
			)

			It("should return error when multiple clients match", func() {
				client := newIdentityClient("my-app", "secret")
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX: &[]api.ClientRepresentation{
							{Id: ptr.To("uid-1")},
							{Id: ptr.To("uid-2")},
						},
					}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("multiple clients found"))
			})

			It("should return error when clientId search returns unexpected status", func() {
				client := newIdentityClient("my-app", "secret")
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{HTTPResponse: httpResp(500)}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
			})

			It("should return error when clientId search returns nil body with 200", func() {
				client := newIdentityClient("my-app", "secret")
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      nil,
					}, nil)

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected empty response body"))
			})

			It("should return error when UID lookup fails with network error", func() {
				client := newIdentityClientWithUID("my-app", "secret", "uuid-123")
				mockClient.EXPECT().GetRealmClientsIdWithResponse(mock.Anything, "realm1", "uuid-123").
					Return(nil, fmt.Errorf("connection refused"))

				err := svc.CreateOrReplaceClient(ctx, "realm1", client, false)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error checking for existing client"))
			})
		})
	})

	// ── CreateOrReplaceRealm ───────────────────────────────────────────

	Describe("CreateOrReplaceRealm", func() {

		It("should create a new realm when it does not exist", func() {
			realm := newRealm("my-realm")
			mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
				Return(&api.GetRealmResponse{HTTPResponse: httpResp(404)}, nil)
			mockClient.EXPECT().PostWithResponse(mock.Anything, mock.Anything).
				Return(&api.PostResponse{HTTPResponse: httpResp(201)}, nil)

			err := svc.CreateOrReplaceRealm(ctx, realm)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should update existing realm when changes detected", func() {
			realm := newRealm("my-realm")
			mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
				Return(&api.GetRealmResponse{
					HTTPResponse: httpResp(200),
					JSON2XX:      &api.RealmRepresentation{Realm: ptr.To("my-realm"), Enabled: ptr.To(false)},
				}, nil)
			mockClient.EXPECT().PutRealmWithResponse(mock.Anything, "my-realm", mock.Anything).
				Return(&api.PutRealmResponse{HTTPResponse: httpResp(204)}, nil)

			err := svc.CreateOrReplaceRealm(ctx, realm)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should skip update when no changes detected", func() {
			realm := newRealm("my-realm")
			mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
				Return(&api.GetRealmResponse{
					HTTPResponse: httpResp(200),
					JSON2XX:      &api.RealmRepresentation{Realm: ptr.To("my-realm"), Enabled: ptr.To(true)},
				}, nil)

			err := svc.CreateOrReplaceRealm(ctx, realm)
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable("should return error on failure",
			func(setup func(), expectedSubstring string) {
				setup()
				realm := newRealm("my-realm")
				err := svc.CreateOrReplaceRealm(ctx, realm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedSubstring))
			},
			Entry("GET realm fails", func() {
				mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
					Return(nil, fmt.Errorf("network error"))
			}, "error checking for existing realm"),
			Entry("POST create returns 500", func() {
				mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
					Return(&api.GetRealmResponse{HTTPResponse: httpResp(404)}, nil)
				mockClient.EXPECT().PostWithResponse(mock.Anything, mock.Anything).
					Return(&api.PostResponse{HTTPResponse: httpResp(500)}, nil)
			}, "creating realm"),
			Entry("PUT update returns 500", func() {
				mockClient.EXPECT().GetRealmWithResponse(mock.Anything, "my-realm").
					Return(&api.GetRealmResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.RealmRepresentation{Realm: ptr.To("my-realm"), Enabled: ptr.To(false)},
					}, nil)
				mockClient.EXPECT().PutRealmWithResponse(mock.Anything, "my-realm", mock.Anything).
					Return(&api.PutRealmResponse{HTTPResponse: httpResp(500)}, nil)
			}, "updating realm"),
		)
	})

	// ── DeleteClient ───────────────────────────────────────────────────

	Describe("DeleteClient", func() {

		It("should skip deletion when client does not exist", func() {
			client := newIdentityClient("my-app", "secret")
			mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
				Return(&api.GetRealmClientsResponse{
					HTTPResponse: httpResp(200),
					JSON2XX:      &[]api.ClientRepresentation{},
				}, nil)

			err := svc.DeleteClient(ctx, "realm1", client)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should delete existing client", func() {
			client := newIdentityClient("my-app", "secret")
			mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
				Return(&api.GetRealmClientsResponse{
					HTTPResponse: httpResp(200),
					JSON2XX:      &[]api.ClientRepresentation{{Id: ptr.To("uid-1"), ClientId: ptr.To("my-app")}},
				}, nil)
			mockClient.EXPECT().DeleteRealmClientsIdWithResponse(mock.Anything, "realm1", "uid-1").
				Return(&api.DeleteRealmClientsIdResponse{HTTPResponse: httpResp(204)}, nil)

			err := svc.DeleteClient(ctx, "realm1", client)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when getClient fails", func() {
			client := newIdentityClient("my-app", "secret")
			mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
				Return(nil, fmt.Errorf("timeout"))

			err := svc.DeleteClient(ctx, "realm1", client)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error checking for existing client"))
		})

		It("should return error when DELETE returns non-204", func() {
			client := newIdentityClient("my-app", "secret")
			mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
				Return(&api.GetRealmClientsResponse{
					HTTPResponse: httpResp(200),
					JSON2XX:      &[]api.ClientRepresentation{{Id: ptr.To("uid-1"), ClientId: ptr.To("my-app")}},
				}, nil)
			mockClient.EXPECT().DeleteRealmClientsIdWithResponse(mock.Anything, "realm1", "uid-1").
				Return(&api.DeleteRealmClientsIdResponse{HTTPResponse: httpResp(500)}, nil)

			err := svc.DeleteClient(ctx, "realm1", client)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deleting client"))
		})
	})

	// ── DeleteRealm ────────────────────────────────────────────────────

	Describe("DeleteRealm", func() {

		DescribeTable("should handle various status codes",
			func(statusCode int, expectErr bool) {
				mockClient.EXPECT().DeleteRealmWithResponse(mock.Anything, "realm1").
					Return(&api.DeleteRealmResponse{HTTPResponse: httpResp(statusCode)}, nil)

				err := svc.DeleteRealm(ctx, "realm1")
				if expectErr {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			},
			Entry("204 No Content", 204, false),
			Entry("404 Not Found (idempotent)", 404, false),
			Entry("500 Server Error", 500, true),
		)

		It("should return error when DELETE fails with network error", func() {
			mockClient.EXPECT().DeleteRealmWithResponse(mock.Anything, "realm1").
				Return(nil, fmt.Errorf("connection refused"))

			err := svc.DeleteRealm(ctx, "realm1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error deleting realm"))
		})
	})

	// ── ConfigureSecretRotationPolicy ──────────────────────────────────

	Describe("ConfigureSecretRotationPolicy", func() {

		policy := &identityv1.SecretRotationConfig{
			ExpirationPeriod:        metav1.Duration{Duration: 29 * 24 * time.Hour},
			GracePeriod:             metav1.Duration{Duration: 1 * time.Hour},
			RemainingRotationPeriod: metav1.Duration{Duration: 10 * 24 * time.Hour},
		}

		Context("when profiles and policies do not exist yet", func() {

			It("should create both profile and policy", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientProfilesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesPoliciesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientPoliciesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesPoliciesResponse{HTTPResponse: httpResp(204)}, nil)

				err := svc.ConfigureSecretRotationPolicy(ctx, "realm1", policy)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when profiles and policies already exist", func() {

			It("should update existing profile and policy in place", func() {
				existingProfile := api.ClientProfileRepresentation{
					Name: ptr.To("controlplane-secret-rotation"),
				}
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX: &api.ClientProfilesRepresentation{
							Profiles: &[]api.ClientProfileRepresentation{existingProfile},
						},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(204)}, nil)

				existingPolicy := api.ClientPolicyRepresentation{
					Name: ptr.To("controlplane-secret-rotation-policy"),
				}
				mockClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesPoliciesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX: &api.ClientPoliciesRepresentation{
							Policies: &[]api.ClientPolicyRepresentation{existingPolicy},
						},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesPoliciesResponse{HTTPResponse: httpResp(204)}, nil)

				err := svc.ConfigureSecretRotationPolicy(ctx, "realm1", policy)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when GET profiles returns nil JSON2XX", func() {

			It("should handle nil response body gracefully", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      nil,
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesPoliciesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      nil,
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesPoliciesResponse{HTTPResponse: httpResp(204)}, nil)

				err := svc.ConfigureSecretRotationPolicy(ctx, "realm1", policy)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		DescribeTable("should return error on failure",
			func(setup func(), expectedSubstring string) {
				setup()
				err := svc.ConfigureSecretRotationPolicy(ctx, "realm1", policy)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedSubstring))
			},
			Entry("GET profiles network error", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(nil, fmt.Errorf("timeout"))
			}, "failed to get client profiles"),
			Entry("GET profiles returns non-200", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(500)}, nil)
			}, "unexpected status getting client profiles"),
			Entry("PUT profiles returns non-204", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientProfilesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(400)}, nil)
			}, "unexpected status putting client profiles"),
			Entry("PUT profiles network error", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientProfilesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(nil, fmt.Errorf("connection refused"))
			}, "failed to put client profiles"),
			Entry("GET policies network error", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientProfilesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(nil, fmt.Errorf("timeout"))
			}, "failed to get client policies"),
			Entry("GET policies returns non-200", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientProfilesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesPoliciesResponse{HTTPResponse: httpResp(500)}, nil)
			}, "unexpected status getting client policies"),
			Entry("PUT policies returns non-204", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientProfilesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesPoliciesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientPoliciesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesPoliciesResponse{HTTPResponse: httpResp(400)}, nil)
			}, "unexpected status putting client policies"),
			Entry("PUT policies network error", func() {
				mockClient.EXPECT().GetRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesProfilesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientProfilesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesProfilesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.PutRealmClientPoliciesProfilesResponse{HTTPResponse: httpResp(204)}, nil)
				mockClient.EXPECT().GetRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientPoliciesPoliciesResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientPoliciesRepresentation{},
					}, nil)
				mockClient.EXPECT().PutRealmClientPoliciesPoliciesWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(nil, fmt.Errorf("connection refused"))
			}, "failed to put client policies"),
		)
	})

	// ── GetClientSecretRotationInfo ────────────────────────────────────

	Describe("GetClientSecretRotationInfo", func() {

		It("should return info with no rotated secret when 404", func() {
			client := newIdentityClient("my-app", "secret")
			mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
				Return(&api.GetRealmClientsResponse{
					HTTPResponse: httpResp(200),
					JSON2XX: &[]api.ClientRepresentation{{
						Id:       ptr.To("uuid-1"),
						ClientId: ptr.To("my-app"),
						Attributes: &map[string]interface{}{
							"client.secret.creation.time": "1750075200",
						},
					}},
				}, nil)
			mockClient.EXPECT().GetRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "uuid-1").
				Return(&api.GetRealmClientsIdClientSecretRotatedResponse{
					HTTPResponse: httpResp(404),
				}, nil)

			info, err := svc.GetClientSecretRotationInfo(ctx, "realm1", client)
			Expect(err).ToNot(HaveOccurred())
			Expect(info).ToNot(BeNil())
			Expect(info.RotatedSecret).To(BeEmpty())
			Expect(info.SecretCreationTime).ToNot(BeNil())
			Expect(*info.SecretCreationTime).To(Equal(int64(1750075200)))
		})

		It("should return rotated secret info when rotation is active", func() {
			client := newIdentityClientWithUID("my-app", "secret", "uuid-1")
			mockClient.EXPECT().GetRealmClientsIdWithResponse(mock.Anything, "realm1", "uuid-1").
				Return(&api.GetRealmClientsIdResponse{
					HTTPResponse: httpResp(200),
					JSON2XX: &api.ClientRepresentation{
						Id:       ptr.To("uuid-1"),
						ClientId: ptr.To("my-app"),
						Attributes: &map[string]interface{}{
							"client.secret.creation.time":           "1750075200",
							"client.secret.rotated.creation.time":   "1750074000",
							"client.secret.rotated.expiration.time": float64(1750078800),
						},
					},
				}, nil)
			mockClient.EXPECT().GetRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "uuid-1").
				Return(&api.GetRealmClientsIdClientSecretRotatedResponse{
					HTTPResponse: httpResp(200),
					JSON2XX:      &api.CredentialRepresentation{Value: ptr.To("old-secret-value")},
				}, nil)

			info, err := svc.GetClientSecretRotationInfo(ctx, "realm1", client)
			Expect(err).ToNot(HaveOccurred())
			Expect(info).ToNot(BeNil())
			Expect(info.RotatedSecret).To(Equal("old-secret-value"))
			Expect(info.SecretCreationTime).ToNot(BeNil())
			Expect(*info.SecretCreationTime).To(Equal(int64(1750075200)))
			Expect(info.RotatedCreatedAt).ToNot(BeNil())
			Expect(*info.RotatedCreatedAt).To(Equal(int64(1750074000)))
			Expect(info.RotatedExpiresAt).ToNot(BeNil())
			Expect(*info.RotatedExpiresAt).To(Equal(int64(1750078800)))
		})

		DescribeTable("should return info with empty rotated secret",
			func(json2xx *api.CredentialRepresentation) {
				client := newIdentityClientWithUID("my-app", "secret", "uuid-1")
				mockClient.EXPECT().GetRealmClientsIdWithResponse(mock.Anything, "realm1", "uuid-1").
					Return(&api.GetRealmClientsIdResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &api.ClientRepresentation{Id: ptr.To("uuid-1"), ClientId: ptr.To("my-app")},
					}, nil)
				mockClient.EXPECT().GetRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "uuid-1").
					Return(&api.GetRealmClientsIdClientSecretRotatedResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      json2xx,
					}, nil)

				info, err := svc.GetClientSecretRotationInfo(ctx, "realm1", client)
				Expect(err).ToNot(HaveOccurred())
				Expect(info).ToNot(BeNil())
				Expect(info.RotatedSecret).To(BeEmpty())
			},
			Entry("nil value in credential", &api.CredentialRepresentation{Value: nil}),
			Entry("empty value in credential", &api.CredentialRepresentation{Value: ptr.To("")}),
			Entry("nil JSON2XX", (*api.CredentialRepresentation)(nil)),
		)

		DescribeTable("should return error on failure",
			func(setup func(), expectedSubstring string) {
				setup()
				client := newIdentityClient("my-app", "secret")
				_, err := svc.GetClientSecretRotationInfo(ctx, "realm1", client)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedSubstring))
			},
			Entry("getClient fails", func() {
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(nil, fmt.Errorf("timeout"))
			}, "failed to look up client"),
			Entry("client not found", func() {
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{},
					}, nil)
			}, "not found in Keycloak"),
			Entry("client has empty ID", func() {
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{{Id: ptr.To(""), ClientId: ptr.To("my-app")}},
					}, nil)
			}, "not found in Keycloak"),
			Entry("rotated secret request fails", func() {
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{{Id: ptr.To("uuid-1"), ClientId: ptr.To("my-app")}},
					}, nil)
				mockClient.EXPECT().GetRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "uuid-1").
					Return(nil, fmt.Errorf("timeout"))
			}, "failed to get rotated secret"),
			Entry("rotated secret returns unexpected status", func() {
				mockClient.EXPECT().GetRealmClientsWithResponse(mock.Anything, "realm1", mock.Anything).
					Return(&api.GetRealmClientsResponse{
						HTTPResponse: httpResp(200),
						JSON2XX:      &[]api.ClientRepresentation{{Id: ptr.To("uuid-1"), ClientId: ptr.To("my-app")}},
					}, nil)
				mockClient.EXPECT().GetRealmClientsIdClientSecretRotatedWithResponse(mock.Anything, "realm1", "uuid-1").
					Return(&api.GetRealmClientsIdClientSecretRotatedResponse{HTTPResponse: httpResp(500)}, nil)
			}, "unexpected status getting rotated secret"),
		)
	})
})
