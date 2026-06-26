// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

const (
	// FileTypeNameLabelKey marks resources related to a FileType name.
	FileTypeNameLabelKey = "filetype.file.ei.telekom.de/name"
	// FileTypeNamespaceLabelKey marks resources related to a FileType namespace.
	FileTypeNamespaceLabelKey = "filetype.file.ei.telekom.de/namespace"
)

// FileSolution defines the backing file transfer solution.
// +kubebuilder:validation:Enum=sftp
type FileSolution string

const (
	FileSolutionSFTP FileSolution = "sftp"
)

func (s FileSolution) OrDefault() FileSolution {
	if s == "" {
		return FileSolutionSFTP
	}
	return s
}

// Visibility defines who can see and subscribe to an exposed file type.
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

// Approval configures how subscriptions to this file exposure are approved.
type Approval struct {
	// Strategy defines the approval mode.
	// +kubebuilder:default=Simple
	Strategy ApprovalStrategy `json:"strategy"`

	// TrustedTeams identifies teams that are trusted for approving subscriptions.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=10
	TrustedTeams []string `json:"trustedTeams,omitempty"`
}

// FileSFTP configures SFTP-specific settings for file exposures and subscriptions.
type FileSFTP struct {
	// PublicKeys contains SSH public keys for the SFTP user of the FileType.
	// +kubebuilder:validation:Optional
	PublicKeys []SSHPublicKeySpec `json:"publicKeys,omitempty"`
}

// SSHPublicKeySpec carries an SSH public key.
type SSHPublicKeySpec struct {
	// Key is the SSH public key value.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}
