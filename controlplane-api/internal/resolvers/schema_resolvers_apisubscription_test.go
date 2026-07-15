// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiSubscription Security", func() {
	var client *ent.Client
	var s *testutil.SeedData

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return security with OAuth2 client credentials", func() {
		ctx := testutil.AllowContext()

		sec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Client: &model.OAuth2ClientCredentials{
					ClientId: "my-client-id",
				},
				Scopes: []string{"read", "write"},
			},
		}

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-oauth").
			SetBasePath("/api/v1/users").
			SetOwner(s.AppAlpha).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodOauth2Client).
			SetSecurity(&sec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiSubscription.Get(ctx, sub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.M2mAuthMethod.String()).To(Equal("OAUTH2_CLIENT"))
		Expect(fetched.Security.M2M).NotTo(BeNil())
		Expect(fetched.Security.M2M.Client).NotTo(BeNil())
		Expect(fetched.Security.M2M.Client.ClientId).To(Equal("my-client-id"))
		Expect(fetched.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
	})

	It("should store and return security with basic auth", func() {
		ctx := testutil.AllowContext()

		sec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Basic: &model.BasicAuthCredentials{
					Username: "test-user",
					Password: "test-pass",
				},
			},
		}

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-basic").
			SetBasePath("/api/v1/basic").
			SetOwner(s.AppAlpha).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodBasicAuth).
			SetSecurity(&sec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiSubscription.Get(ctx, sub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.M2mAuthMethod.String()).To(Equal("BASIC_AUTH"))
		Expect(fetched.Security.M2M).NotTo(BeNil())
		Expect(fetched.Security.M2M.Basic).NotTo(BeNil())
		Expect(fetched.Security.M2M.Basic.Username).To(Equal("test-user"))
		Expect(fetched.Security.M2M.Basic.Password).To(Equal("test-pass"))
		Expect(fetched.Security.M2M.Client).To(BeNil())
	})

	It("should store security with scopes only", func() {
		ctx := testutil.AllowContext()

		sec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Scopes: []string{"admin", "superuser"},
			},
		}

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-scopes").
			SetBasePath("/api/v1/scopes-only").
			SetOwner(s.AppAlpha).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodScopesOnly).
			SetSecurity(&sec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiSubscription.Get(ctx, sub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.M2mAuthMethod.String()).To(Equal("SCOPES_ONLY"))
		Expect(fetched.Security.M2M).NotTo(BeNil())
		Expect(fetched.Security.M2M.Scopes).To(ConsistOf("admin", "superuser"))
		Expect(fetched.Security.M2M.Client).To(BeNil())
		Expect(fetched.Security.M2M.Basic).To(BeNil())
	})

	It("should default to empty security when not set", func() {
		ctx := testutil.AllowContext()

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-default").
			SetBasePath("/api/v1/default").
			SetOwner(s.AppAlpha).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiSubscription.Get(ctx, sub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.M2mAuthMethod.String()).To(Equal("NONE"))
		Expect(fetched.Security).To(BeNil())
	})

	It("should update security from OAuth2 to basic auth", func() {
		ctx := testutil.AllowContext()

		oauthSec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Client: &model.OAuth2ClientCredentials{
					ClientId: "cid",
				},
				Scopes: []string{"read"},
			},
		}

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-update").
			SetBasePath("/api/v1/update").
			SetOwner(s.AppAlpha).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodOauth2Client).
			SetSecurity(&oauthSec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.Security.M2M.Client.ClientId).To(Equal("cid"))

		basicSec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Basic: &model.BasicAuthCredentials{
					Username: "u",
					Password: "p",
				},
				Scopes: []string{"admin"},
			},
		}

		updated, err := client.ApiSubscription.UpdateOneID(sub.ID).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodBasicAuth).
			SetSecurity(&basicSec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.M2mAuthMethod.String()).To(Equal("BASIC_AUTH"))
		Expect(updated.Security.M2M.Basic).NotTo(BeNil())
		Expect(updated.Security.M2M.Basic.Username).To(Equal("u"))
		Expect(updated.Security.M2M.Client).To(BeNil())
		Expect(updated.Security.M2M.Scopes).To(Equal([]string{"admin"}))
	})

	It("should clear security on update", func() {
		ctx := testutil.AllowContext()

		sec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Scopes: []string{"read", "write"},
			},
		}

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-clear").
			SetBasePath("/api/v1/clear").
			SetOwner(s.AppAlpha).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodScopesOnly).
			SetSecurity(&sec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))

		updated, err := client.ApiSubscription.UpdateOneID(sub.ID).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodNone).
			ClearSecurity().
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.M2mAuthMethod.String()).To(Equal("NONE"))
		Expect(updated.Security).To(BeNil())
	})

	It("should update security from basic auth to OAuth2", func() {
		ctx := testutil.AllowContext()

		basicSec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Basic: &model.BasicAuthCredentials{
					Username: "user",
					Password: "pass",
				},
				Scopes: []string{"scope1"},
			},
		}

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-basic-to-oauth").
			SetBasePath("/api/v1/basic-to-oauth").
			SetOwner(s.AppAlpha).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodBasicAuth).
			SetSecurity(&basicSec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.Security.M2M.Basic).NotTo(BeNil())
		Expect(sub.Security.M2M.Client).To(BeNil())

		oauthSec := model.ApiSubscriptionSecurity{
			M2M: &model.SubscriberMachine2MachineAuthentication{
				Client: &model.OAuth2ClientCredentials{
					ClientId: "new-client",
				},
				Scopes: []string{"read", "write"},
			},
		}

		updated, err := client.ApiSubscription.UpdateOneID(sub.ID).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodOauth2Client).
			SetSecurity(&oauthSec).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.M2mAuthMethod.String()).To(Equal("OAUTH2_CLIENT"))
		Expect(updated.Security.M2M.Client).NotTo(BeNil())
		Expect(updated.Security.M2M.Client.ClientId).To(Equal("new-client"))
		Expect(updated.Security.M2M.Basic).To(BeNil())
		Expect(updated.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
	})
})
