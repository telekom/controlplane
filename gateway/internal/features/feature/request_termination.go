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

// RequestTerminationFeature makes the gateway answer every matching request
// with a fixed status code, without calling the upstream.
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

func (f *RequestTerminationFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return route.Spec.RequestTermination != nil
}

func (f *RequestTerminationFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) error {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	builder.RequestTerminationPlugin().Config.StatusCode = int(route.Spec.RequestTermination.StatusCode)
	return nil
}
