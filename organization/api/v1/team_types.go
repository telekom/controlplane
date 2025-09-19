// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"strings"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Member struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=email
	Email string `json:"email"`
}

type TeamCategory string

const (
	TeamCategoryCustomer       TeamCategory = "Customer"
	TeamCategoryInfrastructure TeamCategory = "Infrastructure"
)

// TeamSpec defines the desired state of Team.
type TeamSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name is the name of the team
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=^[a-z0-9]+(-?[a-z0-9]+)*$
	Name string `json:"name"`

	// Group is the group of the team
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=^[a-z0-9]+(-?[a-z0-9]+)*$
	Group string `json:"group"`

	// Email is the mail of the team
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Format=email
	Email string `json:"email"`

	// Members is the members of the team
	// +kubebuilder:validation:MinItems=1
	Members []Member `json:"members"`

	// Secret for the teamToken and passed towards the identity client.
	// +kubebuilder:validation:Optional
	Secret string `json:"secret,omitempty"`

	// Category is the category of the team
	// The category is used to determine specific access rights.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Customer;Infrastructure
	// +kubebuilder:default=Customer
	Category TeamCategory `json:"category"`
}

// TeamStatus defines the observed state of Team.
type TeamStatus struct {
	Namespace          string           `json:"namespace"`
	IdentityClientRef  *types.ObjectRef `json:"identityClientRef,omitempty"`
	GatewayConsumerRef *types.ObjectRef `json:"gatewayConsumerRef,omitempty"`
	// TeamToken is ref for the authentication against teamAPIs
	TeamToken string `json:"teamToken,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

var _ types.Object = &Team{}
var _ types.ObjectList = &TeamList{}

func (t *Team) GetConditions() []metav1.Condition {
	return t.Status.Conditions
}

func (t *Team) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&t.Status.Conditions, condition)

}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Team is the Schema for the teams API.
type Team struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TeamSpec   `json:"spec,omitempty"`
	Status TeamStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TeamList contains a list of Team.
type TeamList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Team `json:"items"`
}

func (tl *TeamList) GetItems() []types.Object {
	items := make([]types.Object, len(tl.Items))
	for i := range tl.Items {
		items[i] = &tl.Items[i]
	}
	return items
}

func init() {
	SchemeBuilder.Register(&Team{}, &TeamList{})
}

// FindTeamForNamespace finds the team for the given namespace.
// The namespace must follow the naming convention <environment>--<group>--<team>.
func FindTeamForNamespace(ctx context.Context, namespace string) (*Team, error) {
	c, ok := cclient.ClientFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("client not found in context")
	}

	parts := strings.Split(namespace, "--")
	if len(parts) != 3 {
		return nil, fmt.Errorf("namespace %q does not follow the naming convention <environment>--<group>--<team>", namespace)
	}

	team := &Team{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parts[0],
			Name:      parts[1] + "--" + parts[2],
		},
	}

	err := c.Get(ctx, client.ObjectKeyFromObject(team), team)
	if err != nil {
		return nil, err
	}

	return team, nil
}

// FindTeamForObject finds the team for the given object.
// The object must be namespaced and the namespace must follow the naming convention <environment>--<group>--<team>.
func FindTeamForObject(ctx context.Context, obj types.NamedObject) (*Team, error) {
	namespace := obj.GetNamespace()
	if namespace == "" {
		return nil, fmt.Errorf("object %q is not namespaced", obj.GetName())
	}
	return FindTeamForNamespace(ctx, namespace)
}
