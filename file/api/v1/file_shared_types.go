// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

// PublicKey is a labeled SSH public key registered on the SFTP user.
type PublicKey struct {
	// Label is a human-readable identifier for the key. It must be unique per file type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Label string `json:"label"`

	// Key is the SSH public key value. It must be unique per file type.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// Visibility defines who can see and subscribe to an exposed file type.
// +kubebuilder:validation:Enum=World;Zone;Enterprise
type Visibility string

const (
	VisibilityWorld      Visibility = "World"
	VisibilityZone       Visibility = "Zone"
	VisibilityEnterprise Visibility = "Enterprise"
)

func (v Visibility) String() string {
	return string(v)
}

// ApprovalStrategy defines the approval mode for subscriptions to a file type exposure.
// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
type ApprovalStrategy string

const (
	ApprovalStrategyAuto     ApprovalStrategy = "Auto"
	ApprovalStrategySimple   ApprovalStrategy = "Simple"
	ApprovalStrategyFourEyes ApprovalStrategy = "FourEyes"
)

func (a ApprovalStrategy) String() string {
	return string(a)
}

// Approval configures how subscriptions to a file type exposure are approved.
type Approval struct {
	// Strategy defines the approval mode for subscriptions to this file type.
	// +kubebuilder:validation:Enum=Auto;Simple;FourEyes
	// +kubebuilder:default=Simple
	Strategy ApprovalStrategy `json:"strategy"`

	// TrustedTeams identifies teams that are trusted for approving subscriptions
	// to this file type. Per default your own team is trusted.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	TrustedTeams []string `json:"trustedTeams,omitempty"`
}
