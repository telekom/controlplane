// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.Feature = &IpRestrictionFeature{}

type IpRestrictionFeature struct {
	priority int
}

var InstanceIpRestrictionFeature = &IpRestrictionFeature{
	priority: 10,
}

func (f *IpRestrictionFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeIpRestriction
}

func (f *IpRestrictionFeature) Priority() int {
	return f.priority
}

func (f *IpRestrictionFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	consumer, ok := builder.GetConsumer()
	if !ok {
		return false
	}

	return consumer.HasIpRestriction()
}

func (f *IpRestrictionFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	consumer, ok := builder.GetConsumer()
	if !ok {
		return features.ErrNoConsumer
	}

	ipRestr := builder.IpRestrictionPlugin()
	for _, allow := range consumer.Spec.Security.IpRestriction.Allow {
		ipRestr.Config.AddAllow(allow)
	}
	for _, deny := range consumer.Spec.Security.IpRestriction.Deny {
		ipRestr.Config.AddDeny(deny)
	}

	return nil
}
