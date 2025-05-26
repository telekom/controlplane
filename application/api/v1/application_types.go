// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type TeamCategory string

const (
	CUSTOMER       TeamCategory = "customer"
	INFRASTRUCTURE TeamCategory = "infrastructure"
)

// ApplicationSpec defines the desired state of Application
type ApplicationSpec struct {
	Team      string          `json:"team"`
	TeamEmail string          `json:"teamEmail"`
	Secret    string          `json:"secret"`
	Zone      types.ObjectRef `json:"zone"`
	// NeedsClient is a flag to indicate if the application needs a Identity client
	// +kubebuilder:default=true
	NeedsClient bool `json:"needsClient"`
	// NeedsConsumer is a flag to indicate if the application needs a Gateway consumer
	// If NeedsClient is true, the consumer will be created for the client
	// +kubebuilder:default=true
	NeedsConsumer bool `json:"needsConsumer"`
}

// ApplicationStatus defines the observed state of Application
type ApplicationStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions   []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	ClientId     string             `json:"clientId"`
	ClientSecret string             `json:"clientSecret"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Application is the Schema for the applications API
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

var _ types.Object = &Application{}

func (r *Application) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *Application) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

var _ types.ObjectList = &ApplicationList{}

func (app *ApplicationList) GetItems() []types.Object {
	items := make([]types.Object, len(app.Items))
	for i := range app.Items {
		items[i] = &app.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
}
