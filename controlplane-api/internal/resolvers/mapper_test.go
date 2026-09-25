// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/zone"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	gqlmodel "github.com/telekom/controlplane/controlplane-api/internal/resolvers/model"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// expectOwnerApplication asserts the reduced ApplicationInfo view mirrors the
// deprecated ownerApplicationName/ownerTeam fields and carries the zone.
func expectOwnerApplication(app *gqlmodel.ApplicationInfo, wantID int, wantName, wantTeam, wantGroup string) {
	GinkgoHelper()
	Expect(app).NotTo(BeNil())
	Expect(app.ID).To(Equal(wantID))
	Expect(app.Name).To(Equal(wantName))
	Expect(app.Zone).NotTo(BeNil())
	Expect(app.Zone.Name).To(Equal("zone-eu"))
	Expect(app.Zone.Visibility).To(Equal(zone.VisibilityEnterprise))
	Expect(app.OwnerTeam).NotTo(BeNil())
	Expect(app.OwnerTeam.Name).To(Equal(wantTeam))
	Expect(app.OwnerTeam.GroupName).To(Equal(wantGroup))
}

var _ = Describe("Subscriptions resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return APISubscriptionInfo for an exposure's subscriptions", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		subs, err := r.APIExposure().Subscriptions(ctx, s.ExposureAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(subs).To(HaveLen(1))
		Expect(subs[0].BasePath).To(Equal("/alpha"))
		Expect(subs[0].OwnerApplicationName).To(Equal("app-beta"))
		Expect(subs[0].OwnerTeam).NotTo(BeNil())
		Expect(subs[0].OwnerTeam.Name).To(Equal("team-beta"))
		Expect(subs[0].OwnerTeam.GroupName).To(Equal("group-b"))
		expectOwnerApplication(subs[0].OwnerApplication, s.AppBeta.ID, "app-beta", "team-beta", "group-b")
	})

	It("should return empty list when no subscriptions exist", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-beta"}})
		subs, err := r.APIExposure().Subscriptions(ctx, s.ExposureBeta)
		Expect(err).NotTo(HaveOccurred())
		Expect(subs).To(BeEmpty())
	})
})

var _ = Describe("Target resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return APIExposureInfo for a subscription's target", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-beta"}})

		client.APIExposure.UpdateOneID(s.ExposureAlpha.ID).SetTraffic(
			model.Traffic{
				RateLimit: &model.RateLimit{
					SubscriberRateLimit: &model.SubscriberRateLimits{
						Overrides: []model.RateLimitOverrides{
							{
								Subscriber: "team-beta",
								Limits: model.Limits{
									Second: 2,
									Minute: 2,
									Hour:   2,
								},
							},
						},
					},
				},
			},
		).SaveX(ctx)

		info, err := r.APISubscription().Target(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		Expect(info.BasePath).To(Equal("/alpha"))
		Expect(info.OwnerApplicationName).To(Equal("app-alpha"))
		Expect(info.OwnerTeam).NotTo(BeNil())
		Expect(info.OwnerTeam.Name).To(Equal("team-alpha"))
		Expect(info.OwnerTeam.GroupName).To(Equal("group-a"))
		expectOwnerApplication(info.OwnerApplication, s.AppAlpha.ID, "app-alpha", "team-alpha", "group-a")
		Expect(info.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Subscriber).To(Equal("team-beta"))
		Expect(info.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Second).To(Equal(2))
		Expect(info.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Minute).To(Equal(2))
		Expect(info.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Hour).To(Equal(2))
	})
})

var _ = Describe("Approval.APISubscription resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return APISubscriptionInfo from an approval", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := r.Approval().Subscription(ctx, s.Approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		apiInfo, ok := info.(*gqlmodel.APISubscriptionInfo)
		Expect(ok).To(BeTrue(), "expected APISubscriptionInfo implementation")
		Expect(apiInfo.BasePath).To(Equal("/alpha"))
		Expect(apiInfo.OwnerApplicationName).To(Equal("app-beta"))
		Expect(apiInfo.OwnerTeam.Name).To(Equal("team-beta"))
		expectOwnerApplication(apiInfo.OwnerApplication, s.AppBeta.ID, "app-beta", "team-beta", "group-b")
	})
})

var _ = Describe("ApprovalRequest.APISubscription resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return APISubscriptionInfo from an approval request", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Admin: true})
		info, err := r.ApprovalRequest().Subscription(ctx, s.ApprovalRequest)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		apiInfo, ok := info.(*gqlmodel.APISubscriptionInfo)
		Expect(ok).To(BeTrue(), "expected APISubscriptionInfo implementation")
		Expect(apiInfo.BasePath).To(Equal("/alpha"))
		Expect(apiInfo.OwnerApplicationName).To(Equal("app-beta"))
		Expect(apiInfo.OwnerTeam.Name).To(Equal("team-beta"))
		expectOwnerApplication(apiInfo.OwnerApplication, s.AppBeta.ID, "app-beta", "team-beta", "group-b")
	})
})

var _ = Describe("APIExposureInfo resolvers", func() {
	r := resolvers.NewResolver(nil, service.Services{}, nil, "")

	It("should convert visibility string to enum", func() {
		v, err := r.APIExposureInfo().Visibility(context.TODO(), &gqlmodel.APIExposureInfo{Visibility: "WORLD"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(v)).To(Equal("WORLD"))
	})

	It("should convert feature strings to enum array", func() {
		features, err := r.APIExposureInfo().Features(context.TODO(), &gqlmodel.APIExposureInfo{
			Features: []string{"BASIC_AUTH", "CIRCUIT_BREAKER"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(features).To(HaveLen(2))
		Expect(string(features[0])).To(Equal("BASIC_AUTH"))
		Expect(string(features[1])).To(Equal("CIRCUIT_BREAKER"))
	})

	It("should return empty features for nil list", func() {
		features, err := r.APIExposureInfo().Features(context.TODO(), &gqlmodel.APIExposureInfo{})
		Expect(err).NotTo(HaveOccurred())
		Expect(features).To(BeEmpty())
	})
})

var _ = Describe("EventExposure.Subscriptions resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return EventSubscriptionInfo for an event exposure's subscriptions", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-alpha"}})
		subs, err := r.EventExposure().Subscriptions(ctx, s.EventExposureAlpha)
		Expect(err).NotTo(HaveOccurred())
		Expect(subs).To(HaveLen(1))
		Expect(subs[0].EventType).To(Equal("order.created"))
		Expect(subs[0].OwnerApplicationName).To(Equal("app-beta"))
		Expect(subs[0].OwnerTeam).NotTo(BeNil())
		Expect(subs[0].OwnerTeam.Name).To(Equal("team-beta"))
		Expect(subs[0].OwnerTeam.GroupName).To(Equal("group-b"))
		expectOwnerApplication(subs[0].OwnerApplication, s.AppBeta.ID, "app-beta", "team-beta", "group-b")
	})
})

var _ = Describe("EventSubscription.Target resolver (cross-tenant)", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
	})

	AfterEach(func() {
		client.Close()
	})

	It("should return EventExposureInfo for an event subscription's target", func() {
		ctx := viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-beta"}})
		info, err := r.EventSubscription().Target(ctx, s.EventSubscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(info).NotTo(BeNil())
		Expect(info.EventType).To(Equal("order.created"))
		Expect(info.OwnerApplicationName).To(Equal("app-alpha"))
		Expect(info.OwnerTeam).NotTo(BeNil())
		Expect(info.OwnerTeam.Name).To(Equal("team-alpha"))
		Expect(info.OwnerTeam.GroupName).To(Equal("group-a"))
		expectOwnerApplication(info.OwnerApplication, s.AppAlpha.ID, "app-alpha", "team-alpha", "group-a")
	})
})

var _ = Describe("EventExposureInfo.Visibility resolver", func() {
	r := resolvers.NewResolver(nil, service.Services{}, nil, "")

	It("should convert visibility string to enum", func() {
		v, err := r.EventExposureInfo().Visibility(context.TODO(), &gqlmodel.EventExposureInfo{Visibility: "WORLD"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(v)).To(Equal("WORLD"))
	})
})

var _ = Describe("APISubscriptionInfo.StatusPhase resolver", func() {
	r := resolvers.NewResolver(nil, service.Services{}, nil, "")

	It("should convert status phase string to enum", func() {
		sp := "SUBSCRIBED"
		phase, err := r.APISubscriptionInfo().StatusPhase(context.TODO(), &gqlmodel.APISubscriptionInfo{StatusPhase: &sp})
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).NotTo(BeNil())
		Expect(string(*phase)).To(Equal("SUBSCRIBED"))
	})

	It("should return nil for nil status phase", func() {
		phase, err := r.APISubscriptionInfo().StatusPhase(context.TODO(), &gqlmodel.APISubscriptionInfo{})
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).To(BeNil())
	})
})
