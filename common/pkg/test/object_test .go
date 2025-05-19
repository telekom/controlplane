// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package test

import (
	"github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ types.Object = &TestResource{}
var _ types.ObjectList = &TestResourceList{}

type TestResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TestResourceSpec   `json:"spec,omitempty"`
	Status TestResourceStatus `json:"status,omitempty"`
}

func (r *TestResource) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

func (r *TestResource) SetCondition(condition metav1.Condition) bool {
	return meta.SetStatusCondition(&r.Status.Conditions, condition)
}

type TestResourceSpec struct {
	Properties *runtime.RawExtension `json:"properties,omitempty"`
}

type TestResourceStatus struct {
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

type TestResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []*TestResource `json:"items"`
}

func (r *TestResourceList) GetItems() []types.Object {
	items := make([]types.Object, len(r.Items))
	for i := range r.Items {
		items[i] = r.Items[i]
	}
	return items
}

func NewObject(name, namespace string) *TestResource {
	return &TestResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "testgroup.cp.ei.telekom.de/v1",
			Kind:       "TestResource",
		},
	}
}

func NewObjectList() *TestResourceList {
	return &TestResourceList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "testgroup.cp.ei.telekom.de/v1",
			Kind:       "TestResourceList",
		},
	}
}
