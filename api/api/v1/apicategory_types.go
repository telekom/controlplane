// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"slices"
	"strings"

	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LintingMode controls how linting failures affect API creation.
// +kubebuilder:validation:Enum=block;warn
type LintingMode string

const (
	// LintingModeBlock prevents Api creation when linting fails.
	LintingModeBlock LintingMode = "block"
	// LintingModeWarn allows Api creation but surfaces linting issues in status.
	LintingModeWarn LintingMode = "warn"
)

// LintingConfig configures OAS specification linting for APIs in this category.
type LintingConfig struct {
	// URL is the base URL of the external linter service.
	// When set, linting is enabled for this category.
	// +kubebuilder:validation:Format=uri
	// +optional
	URL string `json:"url,omitempty"`

	// Ruleset is the name of the linter ruleset to apply.
	// If set, it is passed as a query parameter to the linter API.
	// +optional
	Ruleset string `json:"ruleset,omitempty"`

	// Mode controls how linting failures affect API creation.
	// "block" (default) prevents Api creation on failure; "warn" allows it but surfaces issues.
	// +kubebuilder:validation:Enum=block;warn
	// +kubebuilder:default:=block
	// +optional
	Mode LintingMode `json:"mode,omitempty"`

	// WhitelistedBasepaths is a list of API basepaths that are exempt from linting.
	// APIs whose basePath matches an entry here will skip linting even when a linter URL is configured.
	// +optional
	// +listType=set
	WhitelistedBasepaths []string `json:"whitelistedBasepaths,omitempty"`
}

// ApiCategorySpec defines the desired state of ApiCategory
type ApiCategorySpec struct {
	// LabelValue is the name of the API category in the specification.
	// It must be unique within the cluster.
	// This is the expected value in the info.x-api-category field of the OpenAPI spec
	// +kubebuilder:validation:MaxLength=20
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	LabelValue string `json:"labelValue"`

	// Active indicates whether the API category is active.
	// If false, the API category is not used for new APIs.
	Active bool `json:"active,omitempty"`

	// Description provides a human-readable description of the API category.
	// +kubebuilder:validation:MaxLength=256
	// +optional
	Description string `json:"description,omitempty"`

	AllowTeams *AllowTeamsConfig `json:"allowTeams,omitempty"`

	// MustHaveGroupPrefix indicates whether the API category must have contain
	// the name of the group in the basePath.
	// +kubebuilder:default:=true
	MustHaveGroupPrefix bool `json:"mustHaveGroupPrefix,omitempty"`

	// Linting configures OAS specification linting for APIs in this category.
	// If set with a URL, linting is enabled for this category.
	// +optional
	Linting *LintingConfig `json:"linting,omitempty"`
}

type AllowTeamsConfig struct {
	// Categories defines the list of team categories that are allowed to use this API category.
	// If empty, all team categories are allowed.
	// +optional
	// +listType=set
	Categories []string `json:"categories,omitempty"`
	// Names defines the list of team names that are allowed to use this API category.
	// The name of the team allowed to register an API with this category. Use '*' to allow all teams.
	// +optional
	// +listType=set
	Names []string `json:"names,omitempty"`
}

// ApiCategoryStatus defines the observed state of ApiCategory.
type ApiCategoryStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty,omitzero" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ApiCategory is the Schema for the apicategories API
type ApiCategory struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ApiCategory
	// +required
	Spec ApiCategorySpec `json:"spec"`

	// status defines the observed state of ApiCategory
	// +optional
	Status ApiCategoryStatus `json:"status,omitempty,omitzero"`
}

var _ types.Object = &ApiCategory{}

func (r *ApiCategory) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *ApiCategory) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// IsAllowedForTeamCategory checks if the given team category is allowed to use this ApiCategory.
func (r *ApiCategory) IsAllowedForTeamCategory(teamCategory string) bool {
	if r.Spec.AllowTeams == nil {
		return true
	}
	if slices.Contains(r.Spec.AllowTeams.Categories, "*") {
		return true
	}
	return slices.Contains(r.Spec.AllowTeams.Categories, teamCategory)
}

// IsAllowedForTeam checks if the given team is allowed to use this ApiCategory.
// The provided team should follow the naming convention of "<group>--<team>".
func (r *ApiCategory) IsAllowedForTeam(team string) bool {
	if r.Spec.AllowTeams == nil {
		return true
	}
	if slices.Contains(r.Spec.AllowTeams.Names, "*") {
		return true
	}
	return slices.Contains(r.Spec.AllowTeams.Names, team)
}

// +kubebuilder:object:root=true

// ApiCategoryList contains a list of ApiCategory
type ApiCategoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiCategory `json:"items"`
}

var _ types.ObjectList = &ApiList{}

func (r *ApiCategoryList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = &r.Items[i]
	}
	return items
}

func (r *ApiCategoryList) FindByLabelValue(labelValue string) (*ApiCategory, bool) {
	for i := range r.Items {
		if strings.EqualFold(r.Items[i].Spec.LabelValue, labelValue) {
			return &r.Items[i], true
		}
	}
	return nil, false
}

func (r *ApiCategoryList) AllowedLabelValues() []string {
	values := make([]string, 0, len(r.Items))
	for i := range r.Items {
		if r.Items[i].Spec.Active {
			values = append(values, r.Items[i].Spec.LabelValue)
		}
	}
	slices.Sort(values)
	return values
}

func init() {
	SchemeBuilder.Register(&Api{}, &ApiList{})
}

func init() {
	SchemeBuilder.Register(&ApiCategory{}, &ApiCategoryList{})
}
