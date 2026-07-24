// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package route_test

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
)

type fakeXdsClient struct {
	deletedGateway *gatewayv1.Gateway
	deletedRouteID string
	deleteErr      error
}

func (*fakeXdsClient) PublishRoute(context.Context, *gatewayv1.Gateway, string, []string, envoy.ResourceBundle) error {
	return nil
}

func (f *fakeXdsClient) DeleteRoute(_ context.Context, gateway *gatewayv1.Gateway, routeID string) error {
	f.deletedGateway = gateway
	f.deletedRouteID = routeID
	return f.deleteErr
}
func (f *fakeXdsClient) DeleteRouteWithExpected(ctx context.Context, gateway *gatewayv1.Gateway, routeID string, _ []string) error {
	return f.DeleteRoute(ctx, gateway, routeID)
}

func (*fakeXdsClient) ClearGateway(context.Context, *gatewayv1.Gateway) error { return nil }
