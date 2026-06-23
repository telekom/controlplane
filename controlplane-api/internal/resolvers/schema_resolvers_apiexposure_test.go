// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiExposure.Security", func() {
	var client *ent.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
	})

	AfterEach(func() {
		client.Close()
	})

	It("should store and return security on ApiExposure", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		clientSecret := "test-dummy-client-secret"
		clientKey := "test-dummy-client-key"
		tokenRequest := "client_secret_basic"
		grantType := "client_credentials"
		exposure, err := client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/api/v1/secure").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			SetSecurity(model.ApiExposureSecurity{
				M2M: &model.Machine2MachineAuthentication{
					ExternalIDP: &model.ExternalIdentityProvider{
						TokenEndpoint: "https://idp.example.com/token",
						TokenRequest:  &tokenRequest,
						GrantType:     &grantType,
						Basic: &model.BasicAuthCredentials{
							Username: "test-ext-user",
							Password: "test-dummy-password",
						},
						Client: &model.OAuth2ClientCredentials{
							ClientId:     "ext-client-id",
							ClientSecret: &clientSecret,
							ClientKey:    &clientKey,
						},
					},
					Basic: &model.BasicAuthCredentials{
						Username: "test-svc-user",
						Password: "test-dummy-svc-password",
					},
					Scopes: []string{"read", "write"},
				},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Security.M2M).NotTo(BeNil())
		Expect(fetched.Security.M2M.Basic.Username).To(Equal("test-svc-user"))
		Expect(fetched.Security.M2M.Basic.Password).To(Equal("test-dummy-svc-password"))
		Expect(fetched.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
		Expect(fetched.Security.M2M.ExternalIDP).NotTo(BeNil())
		Expect(fetched.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
		Expect(*fetched.Security.M2M.ExternalIDP.TokenRequest).To(Equal("client_secret_basic"))
		Expect(*fetched.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
		Expect(fetched.Security.M2M.ExternalIDP.Basic.Username).To(Equal("test-ext-user"))
		Expect(fetched.Security.M2M.ExternalIDP.Basic.Password).To(Equal("test-dummy-password"))
		Expect(fetched.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("ext-client-id"))
		Expect(*fetched.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("test-dummy-client-secret"))
		Expect(*fetched.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal("test-dummy-client-key"))
	})

	It("should default to empty security on ApiExposure", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/api/v1/no-sec").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Security.M2M).To(BeNil())
	})

	It("should update security on ApiExposure", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/api/v1/update-sec").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			SetSecurity(model.ApiExposureSecurity{
				M2M: &model.Machine2MachineAuthentication{
					Scopes: []string{"scope1"},
				},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		updated, err := client.ApiExposure.UpdateOneID(exposure.ID).
			SetSecurity(model.ApiExposureSecurity{
				M2M: &model.Machine2MachineAuthentication{
					Basic: &model.BasicAuthCredentials{
						Username: "test-new-user",
						Password: "test-dummy-new-password",
					},
					Scopes: []string{"scope1", "scope2"},
				},
			}).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Security.M2M.Basic.Username).To(Equal("test-new-user"))
		Expect(updated.Security.M2M.Scopes).To(Equal([]string{"scope1", "scope2"}))

		cleared, err := client.ApiExposure.UpdateOneID(exposure.ID).ClearSecurity().Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cleared.Security.M2M).To(BeNil())
	})
})

var _ = Describe("ApiExposure.Traffic", func() {
	var client *ent.Client

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
	})

	AfterEach(func() {
		client.Close()
	})
	It("should store and return traffic on ApiExposure with full RateLimit", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/api/v1/traffic").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			SetTraffic(model.Traffic{
				RateLimit: &model.RateLimit{
					Provider: &model.RateLimitConfig{
						Limits: model.Limits{
							Second: 10,
							Minute: 100,
							Hour:   1000,
						},
						Options: model.RateLimitOptions{
							HideClientHeaders: true,
							FaultTolerant:     true,
						},
					},
					SubscriberRateLimit: &model.SubscriberRateLimits{
						Default: &model.SubscriberRateLimitDefaults{
							Limits: model.Limits{
								Second: 5,
								Minute: 50,
								Hour:   500,
							},
						},
						Overrides: []model.RateLimitOverrides{
							{
								Subscriber: "sub-a",
								Limits:     model.Limits{Second: 20, Minute: 200, Hour: 2000},
							},
						},
					},
				},
				Failover: &model.Failover{
					Zones: []string{"zone-a", "zone-b"},
				},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Traffic.RateLimit).NotTo(BeNil())
		Expect(fetched.Traffic.RateLimit.Provider).NotTo(BeNil())
		Expect(fetched.Traffic.RateLimit.Provider.Limits.Second).To(Equal(10))
		Expect(fetched.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(100))
		Expect(fetched.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(1000))
		Expect(fetched.Traffic.RateLimit.Provider.Options.HideClientHeaders).To(BeTrue())
		Expect(fetched.Traffic.RateLimit.Provider.Options.FaultTolerant).To(BeTrue())
		Expect(fetched.Traffic.RateLimit.SubscriberRateLimit).NotTo(BeNil())
		Expect(fetched.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Second).To(Equal(5))
		Expect(fetched.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Minute).To(Equal(50))
		Expect(fetched.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Hour).To(Equal(500))
		Expect(fetched.Traffic.RateLimit.SubscriberRateLimit.Overrides).To(HaveLen(1))
		Expect(fetched.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Subscriber).To(Equal("sub-a"))
		Expect(fetched.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Second).To(Equal(20))
		Expect(fetched.Traffic.Failover).NotTo(BeNil())
		Expect(fetched.Traffic.Failover.Zones).To(ConsistOf("zone-a", "zone-b"))
	})

	It("should default to empty traffic on ApiExposure", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/api/v1/no-traffic").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Traffic.RateLimit).To(BeNil())
		Expect(fetched.Traffic.Failover).To(BeNil())
	})

	It("should update traffic on ApiExposure", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/api/v1/update-traffic").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			SetTraffic(model.Traffic{
				RateLimit: &model.RateLimit{
					Provider: &model.RateLimitConfig{
						Limits: model.Limits{Second: 1, Minute: 10, Hour: 100},
					},
				},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		updated, err := client.ApiExposure.UpdateOneID(exposure.ID).
			SetTraffic(model.Traffic{
				RateLimit: &model.RateLimit{
					Provider: &model.RateLimitConfig{
						Limits: model.Limits{Second: 99, Minute: 999, Hour: 9999},
						Options: model.RateLimitOptions{
							HideClientHeaders: true,
							FaultTolerant:     true,
						},
					},
				},
				Failover: &model.Failover{
					Zones: []string{"zone-x"},
				},
			}).Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.Traffic.RateLimit.Provider.Limits.Second).To(Equal(99))
		Expect(updated.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(999))
		Expect(updated.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(9999))
		Expect(updated.Traffic.RateLimit.Provider.Options.HideClientHeaders).To(BeTrue())
		Expect(updated.Traffic.Failover.Zones).To(ConsistOf("zone-x"))

		cleared, err := client.ApiExposure.UpdateOneID(exposure.ID).ClearTraffic().Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cleared.Traffic.RateLimit).To(BeNil())
		Expect(cleared.Traffic.Failover).To(BeNil())
	})

	It("should store traffic with only failover zones", func() {
		ctx := testutil.AllowContext()

		zone, err := client.Zone.Create().SetName("zone-eu").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		team, err := client.Team.Create().SetNamespace("default").SetName("team-alpha").SetEmail("a@test.dev").Save(ctx)
		Expect(err).NotTo(HaveOccurred())
		app, err := client.Application.Create().
			SetNamespace("default").SetName("app-alpha").SetClientID("cid-alpha").
			SetOwnerTeam(team).SetZone(zone).Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		exposure, err := client.ApiExposure.Create().
			SetNamespace("default").
			SetBasePath("/api/v1/failover-only").
			SetOwner(app).
			SetApprovalConfig(model.ApprovalConfig{Strategy: "AUTO"}).
			SetTraffic(model.Traffic{
				Failover: &model.Failover{
					Zones: []string{"zone-a", "zone-b", "zone-c"},
				},
			}).
			Save(ctx)
		Expect(err).NotTo(HaveOccurred())

		fetched, err := client.ApiExposure.Get(ctx, exposure.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(fetched.Traffic.RateLimit).To(BeNil())
		Expect(fetched.Traffic.Failover).NotTo(BeNil())
		Expect(fetched.Traffic.Failover.Zones).To(ConsistOf("zone-a", "zone-b", "zone-c"))
	})
})
