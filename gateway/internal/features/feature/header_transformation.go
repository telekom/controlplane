// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ features.Feature = &HeaderTransformationFeature{}

type HeaderTransformationFeature struct {
	priority int
}

var InstanceHeaderTransformationFeature = &HeaderTransformationFeature{
	priority: 0,
}

func (f *HeaderTransformationFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeHeaderTransformation
}

func (f *HeaderTransformationFeature) Priority() int {
	return f.priority
}

func (f *HeaderTransformationFeature) IsUsed(ctx context.Context, builder features.FeaturesBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}

	isPrimaryRoute := !route.IsProxy()
	hasTransformation := route.Spec.Transformation != nil

	return isPrimaryRoute && hasTransformation
}

func (f *HeaderTransformationFeature) Apply(ctx context.Context, builder features.FeaturesBuilder) (err error) {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	RequestTransformerPlugin := builder.RequestTransformerPlugin()

	if route.Spec.Transformation != nil {

		if len(route.Spec.Transformation.Request.Headers.Remove) > 0 {
			// For each header in RemoveHeaders, we add it to the RequestTransformerPlugin
			for _, header := range route.Spec.Transformation.Request.Headers.Remove {
				RequestTransformerPlugin.Config.Remove.AddHeader(header)
			}

		}
	}

	return nil
}
