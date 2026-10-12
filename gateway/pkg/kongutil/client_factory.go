// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

// ClientFactory returns Kong clients for a Gateway.
// The Gateway argument identifies the owner (namespace, name, UID); its connection values
// are not authoritative.
type ClientFactory interface {
	ClientFor(ctx context.Context, gw *gatewayv1.Gateway) (client.KongClient, error)
}

// ClientFactoryFunc adapts a function to ClientFactory.
type ClientFactoryFunc func(ctx context.Context, gw *gatewayv1.Gateway) (client.KongClient, error)

func (f ClientFactoryFunc) ClientFor(ctx context.Context, gw *gatewayv1.Gateway) (client.KongClient, error) {
	return f(ctx, gw)
}
