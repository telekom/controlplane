// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared_test

import (
	"github.com/telekom/controlplane/controlplane-api/pkg/model"
	"github.com/telekom/controlplane/projector/internal/domain/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeriveSubscriptionTraffic", func() {
	It("returns nil when the rate limit is nil", func() {
		Expect(shared.DeriveSubscriptionTraffic(nil, "any")).To(BeNil())
	})

	It("returns nil when neither provider nor subscriber limits apply", func() {
		rl := &model.RateLimit{}
		Expect(shared.DeriveSubscriptionTraffic(rl, "any")).To(BeNil())
	})

	It("maps provider limits only", func() {
		rl := &model.RateLimit{
			Provider: &model.RateLimitConfig{Limits: model.Limits{Second: 50}},
		}
		got := shared.DeriveSubscriptionTraffic(rl, "")
		Expect(got).NotTo(BeNil())
		Expect(got.SubscriberLimits).To(BeNil())
		Expect(*got.ProviderLimits).To(Equal(model.Limits{Second: 50}))
	})

	It("maps the subscriber default when no override matches", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Default: &model.SubscriberRateLimitDefaults{Limits: model.Limits{Second: 10, Minute: 100}},
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "other--team--app", Limits: model.Limits{Second: 1}},
				},
			},
		}
		got := shared.DeriveSubscriptionTraffic(rl, "group--team--app")
		Expect(got).NotTo(BeNil())
		Expect(got.ProviderLimits).To(BeNil())
		Expect(*got.SubscriberLimits).To(Equal(model.Limits{Second: 10, Minute: 100}))
	})

	It("prefers a matching override over the default", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Default: &model.SubscriberRateLimitDefaults{Limits: model.Limits{Second: 10}},
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "group--team--app", Limits: model.Limits{Second: 5, Minute: 50}},
				},
			},
		}
		got := shared.DeriveSubscriptionTraffic(rl, "group--team--app")
		Expect(*got.SubscriberLimits).To(Equal(model.Limits{Second: 5, Minute: 50}))
	})

	It("matches an override even without a default", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "group--team--app", Limits: model.Limits{Second: 7}},
				},
			},
		}
		got := shared.DeriveSubscriptionTraffic(rl, "group--team--app")
		Expect(*got.SubscriberLimits).To(Equal(model.Limits{Second: 7}))
	})

	It("returns nil when only a non-matching override exists", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "other--team--app", Limits: model.Limits{Second: 7}},
				},
			},
		}
		Expect(shared.DeriveSubscriptionTraffic(rl, "group--team--app")).To(BeNil())
	})

	It("combines provider limits with a matching override", func() {
		rl := &model.RateLimit{
			Provider: &model.RateLimitConfig{Limits: model.Limits{Second: 50}},
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Default: &model.SubscriberRateLimitDefaults{Limits: model.Limits{Second: 10}},
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "group--team--app", Limits: model.Limits{Second: 5}},
				},
			},
		}
		got := shared.DeriveSubscriptionTraffic(rl, "group--team--app")
		Expect(*got.ProviderLimits).To(Equal(model.Limits{Second: 50}))
		Expect(*got.SubscriberLimits).To(Equal(model.Limits{Second: 5}))
	})

	It("resolves the default for an empty subscriber id", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Default: &model.SubscriberRateLimitDefaults{Limits: model.Limits{Second: 10}},
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "group--team--app", Limits: model.Limits{Second: 5}},
				},
			},
		}
		got := shared.DeriveSubscriptionTraffic(rl, "")
		Expect(*got.SubscriberLimits).To(Equal(model.Limits{Second: 10}))
	})
})

var _ = Describe("DefaultSubscriptionTraffic", func() {
	It("returns nil when the rate limit is nil", func() {
		Expect(shared.DefaultSubscriptionTraffic(nil)).To(BeNil())
	})

	It("returns nil when neither provider nor default limits apply", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "a--b--c", Limits: model.Limits{Second: 5}},
				},
			},
		}
		Expect(shared.DefaultSubscriptionTraffic(rl)).To(BeNil())
	})

	It("returns the subscriber default, ignoring any overrides", func() {
		rl := &model.RateLimit{
			Provider: &model.RateLimitConfig{Limits: model.Limits{Second: 50}},
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Default: &model.SubscriberRateLimitDefaults{Limits: model.Limits{Second: 10}},
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "a--b--c", Limits: model.Limits{Second: 5}},
				},
			},
		}
		got := shared.DefaultSubscriptionTraffic(rl)
		Expect(*got.ProviderLimits).To(Equal(model.Limits{Second: 50}))
		Expect(*got.SubscriberLimits).To(Equal(model.Limits{Second: 10}))
	})

	It("matches DeriveSubscriptionTraffic for an unknown subscriber", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Default: &model.SubscriberRateLimitDefaults{Limits: model.Limits{Second: 10}},
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "a--b--c", Limits: model.Limits{Second: 5}},
				},
			},
		}
		Expect(shared.DefaultSubscriptionTraffic(rl)).To(Equal(shared.DeriveSubscriptionTraffic(rl, "no-such-subscriber")))
	})
})

var _ = Describe("SubscriberOverrideIDs", func() {
	It("returns nil for a nil rate limit", func() {
		Expect(shared.SubscriberOverrideIDs(nil)).To(BeNil())
	})

	It("returns nil when there is no subscriber rate limit", func() {
		Expect(shared.SubscriberOverrideIDs(&model.RateLimit{})).To(BeNil())
	})

	It("returns nil when there are no overrides", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Default: &model.SubscriberRateLimitDefaults{Limits: model.Limits{Second: 10}},
			},
		}
		Expect(shared.SubscriberOverrideIDs(rl)).To(BeNil())
	})

	It("returns the override subscriber ids in order", func() {
		rl := &model.RateLimit{
			SubscriberRateLimit: &model.SubscriberRateLimits{
				Overrides: []model.RateLimitOverrides{
					{Subscriber: "a--b--c", Limits: model.Limits{Second: 1}},
					{Subscriber: "d--e--f", Limits: model.Limits{Second: 2}},
				},
			},
		}
		Expect(shared.SubscriberOverrideIDs(rl)).To(Equal([]string{"a--b--c", "d--e--f"}))
	})
})
