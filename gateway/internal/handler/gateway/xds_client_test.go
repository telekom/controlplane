// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_test

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
)

type fakeXdsClient struct {
	clearedGateway *gatewayv1.Gateway
	clearErr       error
}

func (*fakeXdsClient) PublishRoute(context.Context, *gatewayv1.Gateway, string, []string, envoy.ResourceBundle) error {
	return nil
}

func (*fakeXdsClient) DeleteRoute(context.Context, *gatewayv1.Gateway, string) error { return nil }
func (*fakeXdsClient) DeleteRouteWithExpected(context.Context, *gatewayv1.Gateway, string, []string) error {
	return nil
}

func (f *fakeXdsClient) ClearGateway(_ context.Context, gateway *gatewayv1.Gateway) error {
	f.clearedGateway = gateway
	return f.clearErr
}
