// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import "github.com/telekom/controlplane/common/pkg/util/labelutil"

const (
	// AiGatewayRoutePrefix is prepended to MCP route names for namespace isolation.
	AiGatewayRoutePrefix = "ai-gateway"

	// GatewayConsumerName is the consumer name for cross-zone proxy access.
	// Must match the API domain's GatewayConsumerName constant.
	GatewayConsumerName = "gateway"
)

// MakeMcpRouteName creates the route name for an MCP exposure.
// Format: "ai-gateway--<normalized-basepath>"
func MakeMcpRouteName(basePath string) string {
	return AiGatewayRoutePrefix + "--" + labelutil.NormalizeNameValue(basePath)
}
