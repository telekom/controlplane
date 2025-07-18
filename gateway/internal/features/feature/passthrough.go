// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.Feature = &PassThroughFeature{}

type PassThroughFeature struct {
	priority int
}

var InstancePassThroughFeature = &PassThroughFeature{
	priority: 0,
}

func (f *PassThroughFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypePassThrough
}

func (f *PassThroughFeature) Priority() int {
	return f.priority
}

func (f *PassThroughFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return len(route.Spec.Upstreams) > 0 && route.Spec.PassThrough
}

func (f *PassThroughFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	builder.SetUpstream(route.Spec.Upstreams[0])

	return nil
}
