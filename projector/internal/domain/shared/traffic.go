// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared

import "github.com/telekom/controlplane/controlplane-api/pkg/model"

// DeriveSubscriptionTraffic flattens an exposure's rate-limit configuration
// into the per-subscriber view stored on an ApiSubscription.
//
// subscriberID is the consumer's client id ("<group>--<team>--<app>") — the
// same identifier the gateway matches subscriber overrides against. A subscriber
// with a matching override gets those limits; anyone else gets the default. Use
// DefaultSubscriptionTraffic when there is no specific subscriber to match.
// Returns nil when neither provider nor subscriber limits apply, which callers
// persist as a NULL traffic column.
func DeriveSubscriptionTraffic(rl *model.RateLimit, subscriberID string) *model.ApiSubscriptionTraffic {
	if rl == nil {
		return nil
	}
	subLimits := subscriberDefaultLimits(rl)
	if override := subscriberOverrideLimits(rl, subscriberID); override != nil {
		subLimits = override
	}
	return assembleTraffic(rl, subLimits)
}

// DefaultSubscriptionTraffic derives the traffic for a subscriber that has no
// per-subscriber override: provider limits plus the subscriber default.
func DefaultSubscriptionTraffic(rl *model.RateLimit) *model.ApiSubscriptionTraffic {
	if rl == nil {
		return nil
	}
	return assembleTraffic(rl, subscriberDefaultLimits(rl))
}

// subscriberDefaultLimits returns the default subscriber limits, or nil if none.
func subscriberDefaultLimits(rl *model.RateLimit) *model.Limits {
	if rl.SubscriberRateLimit != nil && rl.SubscriberRateLimit.Default != nil {
		return &rl.SubscriberRateLimit.Default.Limits
	}
	return nil
}

// subscriberOverrideLimits returns the limits of the override matching
// subscriberID, or nil when no override matches.
func subscriberOverrideLimits(rl *model.RateLimit, subscriberID string) *model.Limits {
	if rl.SubscriberRateLimit == nil {
		return nil
	}
	for i := range rl.SubscriberRateLimit.Overrides {
		if rl.SubscriberRateLimit.Overrides[i].Subscriber == subscriberID {
			return &rl.SubscriberRateLimit.Overrides[i].Limits
		}
	}
	return nil
}

// assembleTraffic combines provider limits with the resolved subscriber limits,
// returning nil when neither applies.
func assembleTraffic(rl *model.RateLimit, subscriber *model.Limits) *model.ApiSubscriptionTraffic {
	var provider *model.Limits
	if rl.Provider != nil {
		provider = &rl.Provider.Limits
	}
	if provider == nil && subscriber == nil {
		return nil
	}
	return &model.ApiSubscriptionTraffic{SubscriberLimits: subscriber, ProviderLimits: provider}
}

// SubscriberOverrideIDs returns the subscriber client ids that carry an explicit
// rate-limit override. Returns nil when the config declares none, letting
// callers skip the (otherwise unnecessary) client-id resolution.
func SubscriberOverrideIDs(rl *model.RateLimit) []string {
	if rl == nil || rl.SubscriberRateLimit == nil || len(rl.SubscriberRateLimit.Overrides) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rl.SubscriberRateLimit.Overrides))
	for i := range rl.SubscriberRateLimit.Overrides {
		ids = append(ids, rl.SubscriberRateLimit.Overrides[i].Subscriber)
	}
	return ids
}
