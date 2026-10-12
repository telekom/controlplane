// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import "github.com/telekom/controlplane/common/pkg/util/labelutil"

const (
	// AiGatewayRoutePrefix is prepended to MCP route names for namespace isolation.
	AiGatewayRoutePrefix = "ai-gateway"

	// GatewayConsumerName is the consumer name of the gateway mesh client used for cross-zone
	// proxy access. Only Routes that are the target of a proxy Route must trust it.
	// Must match the API domain's GatewayConsumerName constant.
	GatewayConsumerName = "gateway"
)

const (
	LabelValueDomain = "agentic"
)

// MakeAgenticRouteName creates the route name for an MCP exposure.
// Format: "ai-gateway--<normalized-basepath>"
func MakeAgenticRouteName(basePath string) string {
	return labelutil.NormalizeNameValue(AiGatewayRoutePrefix + "--" + labelutil.NormalizeValue(basePath))
}
