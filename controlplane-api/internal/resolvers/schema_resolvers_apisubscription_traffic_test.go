// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package resolvers_test

import (
	"context"

	"github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/internal/resolvers"
	"github.com/telekom/controlplane/controlplane-api/internal/service"
	"github.com/telekom/controlplane/controlplane-api/internal/testutil"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
	"github.com/telekom/controlplane/controlplane-api/pkg/model"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The seeded subscription (s.Subscription) is owned by AppBeta. Its client ID
// follows the real MakeClientName format "<team>--<app>" (team-beta--app-beta).
// Subscriber overrides are matched by that client ID, exactly like the gateway's
// GetOverriddenSubscriberRateLimit.
const subscriberClientID = "team-beta--app-beta"

var _ = Describe("ApiSubscription Traffic resolver", func() {
	var (
		client *ent.Client
		r      *resolvers.Resolver
		s      *testutil.SeedData
		ctx    context.Context
	)

	BeforeEach(func() {
		client = testutil.NewTestClient(GinkgoT())
		r = resolvers.NewResolver(client, service.Services{}, nil, "")
		s = testutil.SeedStandard(client)
		// Seed hardcodes a placeholder client ID; use the real team--app format.
		client.Application.UpdateOneID(s.AppBeta.ID).SetClientID(subscriberClientID).SaveX(testutil.AllowContext())
		ctx = viewer.NewContext(testutil.AllowContext(), &viewer.Viewer{Teams: []string{"team-beta"}})
	})

	AfterEach(func() {
		client.Close()
	})

	setExposureTraffic := func(traffic model.Traffic) {
		client.ApiExposure.UpdateOneID(s.ExposureAlpha.ID).SetTraffic(traffic).SaveX(testutil.AllowContext())
	}

	It("should return nil when the exposure has no rate limit config", func() {
		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).To(BeNil())
	})

	It("should expose provider limits when only provider rate limit is configured", func() {
		setExposureTraffic(model.Traffic{
			RateLimit: &model.RateLimit{
				Provider: &model.RateLimitConfig{Limits: model.Limits{Second: 50, Minute: 500, Hour: 5000}},
			},
		})

		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).NotTo(BeNil())
		Expect(traffic.ProviderLimits).NotTo(BeNil())
		Expect(*traffic.ProviderLimits).To(Equal(model.Limits{Second: 50, Minute: 500, Hour: 5000}))
		Expect(traffic.SubscriberLimits).To(BeNil())
	})

	It("should expose both provider and subscriber limits", func() {
		setExposureTraffic(model.Traffic{
			RateLimit: &model.RateLimit{
				Provider: &model.RateLimitConfig{Limits: model.Limits{Second: 50}},
				SubscriberRateLimit: &model.SubscriberRateLimits{
					Overrides: []model.RateLimitOverrides{
						{Subscriber: subscriberClientID, Limits: model.Limits{Second: 5}},
					},
				},
			},
		})

		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).NotTo(BeNil())
		Expect(traffic.ProviderLimits).NotTo(BeNil())
		Expect(*traffic.ProviderLimits).To(Equal(model.Limits{Second: 50}))
		Expect(traffic.SubscriberLimits).NotTo(BeNil())
		Expect(*traffic.SubscriberLimits).To(Equal(model.Limits{Second: 5}))
	})

	It("should apply the default subscriber limits when only a default is configured", func() {
		setExposureTraffic(model.Traffic{
			RateLimit: &model.RateLimit{
				SubscriberRateLimit: &model.SubscriberRateLimits{
					Default: &model.SubscriberRateLimitDefaults{
						Limits: model.Limits{Second: 10, Minute: 100, Hour: 1000},
					},
				},
			},
		})

		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).NotTo(BeNil())
		Expect(traffic.SubscriberLimits).NotTo(BeNil())
		Expect(*traffic.SubscriberLimits).To(Equal(model.Limits{Second: 10, Minute: 100, Hour: 1000}))
		Expect(traffic.ProviderLimits).To(BeNil())
	})

	It("should prefer a matching subscriber override (by client ID) over the default", func() {
		setExposureTraffic(model.Traffic{
			RateLimit: &model.RateLimit{
				SubscriberRateLimit: &model.SubscriberRateLimits{
					Default: &model.SubscriberRateLimitDefaults{
						Limits: model.Limits{Second: 10, Minute: 100, Hour: 1000},
					},
					Overrides: []model.RateLimitOverrides{
						{Subscriber: "other-client", Limits: model.Limits{Second: 1}},
						{Subscriber: subscriberClientID, Limits: model.Limits{Second: 5, Minute: 50, Hour: 500}},
					},
				},
			},
		})

		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).NotTo(BeNil())
		Expect(traffic.SubscriberLimits).NotTo(BeNil())
		Expect(*traffic.SubscriberLimits).To(Equal(model.Limits{Second: 5, Minute: 50, Hour: 500}))
	})

	It("should apply the default when overrides exist but none match", func() {
		setExposureTraffic(model.Traffic{
			RateLimit: &model.RateLimit{
				SubscriberRateLimit: &model.SubscriberRateLimits{
					Default: &model.SubscriberRateLimitDefaults{
						Limits: model.Limits{Second: 10, Minute: 100, Hour: 1000},
					},
					Overrides: []model.RateLimitOverrides{
						{Subscriber: "other-client", Limits: model.Limits{Second: 1}},
						{Subscriber: "another-client", Limits: model.Limits{Second: 2}},
					},
				},
			},
		})

		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).NotTo(BeNil())
		Expect(traffic.SubscriberLimits).NotTo(BeNil())
		Expect(*traffic.SubscriberLimits).To(Equal(model.Limits{Second: 10, Minute: 100, Hour: 1000}))
	})

	It("should apply an override even when no default is configured", func() {
		setExposureTraffic(model.Traffic{
			RateLimit: &model.RateLimit{
				SubscriberRateLimit: &model.SubscriberRateLimits{
					Overrides: []model.RateLimitOverrides{
						{Subscriber: subscriberClientID, Limits: model.Limits{Second: 7}},
					},
				},
			},
		})

		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).NotTo(BeNil())
		Expect(traffic.SubscriberLimits).NotTo(BeNil())
		Expect(*traffic.SubscriberLimits).To(Equal(model.Limits{Second: 7}))
	})

	It("should return nil when only a non-matching override exists", func() {
		setExposureTraffic(model.Traffic{
			RateLimit: &model.RateLimit{
				SubscriberRateLimit: &model.SubscriberRateLimits{
					Overrides: []model.RateLimitOverrides{
						{Subscriber: "other-client", Limits: model.Limits{Second: 3}},
					},
				},
			},
		})

		traffic, err := r.ApiSubscription().Traffic(ctx, s.Subscription)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).To(BeNil())
	})

	It("should return nil when the subscription has no target exposure", func() {
		sub := client.ApiSubscription.Create().
			SetNamespace("default").
			SetName("sub-no-target").
			SetBasePath("/no-target").
			SetOwner(s.AppBeta).
			SaveX(testutil.AllowContext())

		traffic, err := r.ApiSubscription().Traffic(ctx, sub)
		Expect(err).NotTo(HaveOccurred())
		Expect(traffic).To(BeNil())
	})
})
