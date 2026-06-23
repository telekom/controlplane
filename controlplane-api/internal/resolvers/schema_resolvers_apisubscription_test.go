// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiSubscription.M2MAuthMethod", func() {
	var client *ent.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return m2m_auth_method and approved_scopes", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-oauth").
			SetBasePath("/api/v1/users").
			SetOwner(app).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodOauth2Client).
			SetApprovedScopes([]string{"read", "write"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiSubscription.Get(ctx, sub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.M2mAuthMethod.String()).To(Equal("OAUTH2_CLIENT"))
		Expect(fetched.ApprovedScopes).To(Equal([]string{"read", "write"}))
	})

	It("should default to NONE and empty scopes", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-default").
			SetBasePath("/api/v1/default").
			SetOwner(app).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiSubscription.Get(ctx, sub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.M2mAuthMethod.String()).To(Equal("NONE"))
		Expect(fetched.ApprovedScopes).To(Equal([]string{}))
	})

	It("should update m2m_auth_method from OAUTH2_CLIENT to BASIC_AUTH", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-update").
			SetBasePath("/api/v1/update-auth").
			SetOwner(app).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodOauth2Client).
			SetApprovedScopes([]string{"read"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.M2mAuthMethod.String()).To(Equal("OAUTH2_CLIENT"))
		Expect(sub.ApprovedScopes).To(Equal([]string{"read"}))

		updated, err := client.ApiSubscription.UpdateOneID(sub.ID).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodBasicAuth).
			SetApprovedScopes([]string{"admin"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.M2mAuthMethod.String()).To(Equal("BASIC_AUTH"))
		Expect(updated.ApprovedScopes).To(Equal([]string{"admin"}))
	})

	It("should update from BASIC_AUTH to OAUTH2_CLIENT", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-switch").
			SetBasePath("/api/v1/switch-auth").
			SetOwner(app).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodBasicAuth).
			SetApprovedScopes([]string{"scope1"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.M2mAuthMethod.String()).To(Equal("BASIC_AUTH"))
		Expect(sub.ApprovedScopes).To(Equal([]string{"scope1"}))

		updated, err := client.ApiSubscription.UpdateOneID(sub.ID).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodOauth2Client).
			SetApprovedScopes([]string{"read", "write"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.M2mAuthMethod.String()).To(Equal("OAUTH2_CLIENT"))
		Expect(updated.ApprovedScopes).To(Equal([]string{"read", "write"}))
	})

	It("should clear scopes and reset to NONE", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-clear").
			SetBasePath("/api/v1/clear-auth").
			SetOwner(app).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodScopesOnly).
			SetApprovedScopes([]string{"read", "write"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(sub.M2mAuthMethod.String()).To(Equal("SCOPES_ONLY"))
		Expect(sub.ApprovedScopes).To(Equal([]string{"read", "write"}))

		updated, err := client.ApiSubscription.UpdateOneID(sub.ID).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodNone).
			SetApprovedScopes([]string{}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.M2mAuthMethod.String()).To(Equal("NONE"))
		Expect(updated.ApprovedScopes).To(Equal([]string{}))
	})

	It("should support SCOPES_ONLY auth method", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		sub, err := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-scopes").
			SetBasePath("/api/v1/scopes-only").
			SetOwner(app).
			SetM2mAuthMethod(apisubscription.M2mAuthMethodScopesOnly).
			SetApprovedScopes([]string{"admin", "superuser"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiSubscription.Get(ctx, sub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.M2mAuthMethod.String()).To(Equal("SCOPES_ONLY"))
		Expect(fetched.ApprovedScopes).To(ConsistOf("admin", "superuser"))
	})
})
