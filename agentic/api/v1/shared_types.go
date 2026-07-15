// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import "github.com/telekom/controlplane/common/pkg/config"

// McpVariant defines the MCP exposure variant.
// +kubebuilder:validation:Enum=MCP;TELECONTEXTMCP
type McpVariant string

const (
	McpVariantMCP            McpVariant = "MCP"
	McpVariantTelecontextMCP McpVariant = "TELECONTEXTMCP"
)

// IsTelecontextVariant returns true if the variant requires automatic Telecontext integration.
func (v McpVariant) IsTelecontextVariant() bool {
	return v == McpVariantTelecontextMCP
}

// Visibility defines who can see and subscribe to an exposed MCP server.
// +kubebuilder:validation:Enum=World;Zone;Enterprise
type Visibility string

const (
	VisibilityWorld      Visibility = "World"
	VisibilityZone       Visibility = "Zone"
	VisibilityEnterprise Visibility = "Enterprise"
)

// ApprovalStrategy defines the approval mode for subscriptions.
// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
type ApprovalStrategy string

const (
	ApprovalStrategyAuto     ApprovalStrategy = "Auto"
	ApprovalStrategySimple   ApprovalStrategy = "Simple"
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

// Approval configures how subscriptions to this MCP server are approved.
type Approval struct {
	// Strategy defines the approval mode.
	// +kubebuilder:default=Auto
	Strategy ApprovalStrategy `json:"strategy"`

	// TrustedTeams identifies teams that are trusted for approving subscriptions.
	// +optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	TrustedTeams []string `json:"trustedTeams,omitempty"`
}

// Label keys used for agentic domain resources.
var (
	McpBasePathLabelKey = config.BuildLabelKey("mcpbasepath")
)
