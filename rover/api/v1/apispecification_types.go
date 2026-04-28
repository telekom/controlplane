// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Major extracts the major version from a semantic version string formatted as "major.minor.patch".
func Major(v string) string {
	parts := strings.Split(string(v), ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return string(v)
}

// MakeName generates a name for the Api resource based on the BasePath of the ApiSpecification.
// It normalizes the BasePath by removing leading/trailing slashes and replacing slashes with hyphens.
func MakeName(apiSpec *ApiSpecification) string {
	basePath := strings.Trim(apiSpec.Spec.BasePath, "/")
	name := strings.ReplaceAll(basePath, "/", "-")
	return strings.ToLower(name)
}

type ApiSpecificationSpec struct {
	// Specification contains the file ID reference from the file manager
	// +kubebuilder:validation:Required
	Specification string `json:"specification"`

	// Category of the API, defaults to "other" if not specified, is extracted from `x-api-category` in rover
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=other
	Category string `json:"category"`

	// BasePath represents the base path from OpenAPI v2 or derived from server URL in OpenAPI v3
	// It must start with a "/" and have a maximum length of 200 characters (to avoid Kubernetes name length issues)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^/.*$`
	// +kubebuilder:validation:MaxLength:=200
	BasePath string `json:"basepath"`

	// Hash is the SHA-256 hash of the specification content for integrity verification
	// +kubebuilder:validation:Required
	Hash string `json:"hash"`

	// XVendor indicates if this is a vendor extension API, defaults to false is extracted from `x-vendor` in rover-server
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	XVendor bool `json:"xvendor"`

	// Version of the API as specified in the OpenAPI document's info section
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\d+.*$`
	Version string `json:"version"`

	// Oauth2Scopes contains the OAuth2 scopes extracted from security definitions/schemes
	// +kubebuilder:validation:Optional
	Oauth2Scopes []string `json:"scopes,omitempty"`
}

type ApiSpecificationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// API reference
	Api types.ObjectRef `json:"api,omitempty"`

	// LintedHash is the spec hash that was last linted. Compared with Spec.Hash to avoid re-linting.
	// +optional
	LintedHash string `json:"lintedHash,omitempty"`

	// LintPassed indicates whether the last lint passed. nil means not yet linted.
	// +optional
	LintPassed *bool `json:"lintPassed,omitempty"`

	// LintReason is a human-readable message describing the lint outcome.
	// +optional
	LintReason string `json:"lintReason,omitempty"`

	// LinterId is the scan ID returned by the external linter API.
	// +optional
	LinterId string `json:"linterId,omitempty"`

	// LintRuleset is the name of the ruleset used for linting.
	// +optional
	LintRuleset string `json:"lintRuleset,omitempty"`

	// LintLinterVersion is the version of the linter engine.
	// +optional
	LintLinterVersion string `json:"lintLinterVersion,omitempty"`

	// LintErrors is the number of errors found during linting.
	// +optional
	LintErrors int `json:"lintErrors,omitempty"`

	// LintWarnings is the number of warnings found during linting.
	// +optional
	LintWarnings int `json:"lintWarnings,omitempty"`

	// LintDashboardURL is a direct link to the linter dashboard for this scan.
	// Populated from the zone's DashboardURLTemplate with the scan ID substituted.
	// +optional
	LintDashboardURL string `json:"lintDashboardUrl,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ApiSpecification is the Schema for the apispecifications API
// +kubebuilder:pruning:PreserveUnknownFields
type ApiSpecification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiSpecificationSpec   `json:"spec,omitempty"`
	Status ApiSpecificationStatus `json:"status,omitempty"`
}

var _ types.Object = &ApiSpecification{}

func (r *ApiSpecification) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *ApiSpecification) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

//+kubebuilder:object:root=true

type ApiSpecificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiSpecification `json:"items"`
}

var _ types.ObjectList = &ApiSpecificationList{}

func (r *ApiSpecificationList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items = append(items, &r.Items[i])
	}
	return items
}

func init() {
	SchemeBuilder.Register(&ApiSpecification{}, &ApiSpecificationList{})
}
