// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.Feature = &RequestTerminationFeature{}

type RequestTerminationFeature struct {
	priority int
}

var InstanceRequestTerminationFeature = &RequestTerminationFeature{
	priority: 0,
}

func (f *RequestTerminationFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeRequestTermination
}

func (f *RequestTerminationFeature) Priority() int {
	return f.priority
}

func (f *RequestTerminationFeature) IsUsed(_ context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	return ok && route.Spec.Traffic.HealthProbe
}

func (f *RequestTerminationFeature) Apply(_ context.Context, builder features.FeaturesBuilder) error {
	if _, ok := builder.GetRoute(); !ok {
		return features.ErrNoRoute
	}
	builder.RequestTerminationPlugin()
	return nil
}
