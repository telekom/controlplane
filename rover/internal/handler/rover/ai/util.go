// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
)

// MakeName generates a deterministic resource name for an AI exposure or subscription.
// It combines the owner (application) name with the normalized MCP server name.
func MakeName(ownerName, basePath string) string {
	return ownerName + "--" + agenticv1.MakeMcpServerName(basePath)
}
