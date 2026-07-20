// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import "github.com/telekom/controlplane/common/pkg/config"

// AgenticVariant defines the agentic exposure variant.
// +kubebuilder:validation:Enum=MCP;TELECONTEXTMCP;AGENT
type AgenticVariant string

const (
	AgenticVariantMCP            AgenticVariant = "MCP"
	AgenticVariantTelecontextMCP AgenticVariant = "TELECONTEXTMCP"
	AgenticVariantAgent          AgenticVariant = "AGENT"
)

// IsTelecontextVariant returns true if the variant requires automatic Telecontext integration.
func (v AgenticVariant) IsTelecontextVariant() bool {
	return v == AgenticVariantTelecontextMCP
}

// IsAgentVariant returns true if the variant represents an A2A agent.
func (v AgenticVariant) IsAgentVariant() bool {
	return v == AgenticVariantAgent
}

// DisplayType returns the display type for approval resource kind differentiation.
func (v AgenticVariant) DisplayType() string {
	switch v {
	case AgenticVariantAgent:
		return "AGENT"
	default:
		return "MCP"
	}
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
	AgenticBasePathLabelKey = config.BuildLabelKey("mcpbasepath")
)
