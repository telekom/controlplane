// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// ObjectRef is a reference to a Kubernetes object
// It is similar to types.NamespacedName but has the required json tags for serialization
type ObjectRef struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	UID       types.UID `json:"uid,omitempty"`
}

var _ NamedObject = &ObjectRef{}

type NamedObject interface {
	GetName() string
	GetNamespace() string
}

func (o *ObjectRef) GetName() string {
	return o.Name
}

func (o *ObjectRef) GetNamespace() string {
	return o.Namespace
}

func (o *ObjectRef) K8s() client.ObjectKey {
	return client.ObjectKey{
		Name:      o.Name,
		Namespace: o.Namespace,
	}
}

func (o *ObjectRef) Equals(other NamedObject) bool {
	return o.Name == other.GetName() && o.Namespace == other.GetNamespace()
}

func (o *ObjectRef) DeepCopy() *ObjectRef {
	return &ObjectRef{
		Name:      o.Name,
		Namespace: o.Namespace,
	}
}

func (o *ObjectRef) DeepCopyInto(out *ObjectRef) {
	*out = *o
}

func (o ObjectRef) String() string {
	return o.Namespace + "/" + o.Name
}

func ObjectRefFromObject(obj client.Object) *ObjectRef {
	return &ObjectRef{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		UID:       obj.GetUID(),
	}
}

// TypedObjectRef is a reference to a Kubernetes object with type information
// It is similar to ObjectRef but includes the Kind and APIVersion
type TypedObjectRef struct {
	metav1.TypeMeta `json:",inline"`
	ObjectRef       `json:",inline"`
}

type TypedNamedObject interface {
	NamedObject
	GetKind() string
	GetAPIVersion() string
}

var _ TypedNamedObject = &TypedObjectRef{}

func (o *TypedObjectRef) GetKind() string {
	return o.Kind
}

func (o *TypedObjectRef) GetAPIVersion() string {
	return o.APIVersion
}

func TypedObjectRefFromObject(obj client.Object, scheme *runtime.Scheme) *TypedObjectRef {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		panic(err)
	}
	return &TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			Kind:       gvk.Kind,
			APIVersion: gvk.GroupVersion().String(),
		},
		ObjectRef: ObjectRef{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			UID:       obj.GetUID(),
		},
	}
}

func (o *TypedObjectRef) Equals(other TypedNamedObject) bool {
	return o.Kind == other.GetKind() &&
		o.APIVersion == other.GetAPIVersion() &&
		o.ObjectRef.Equals(other)
}
func (o *TypedObjectRef) DeepCopy() *TypedObjectRef {
	return &TypedObjectRef{
		TypeMeta:  o.TypeMeta,
		ObjectRef: *o.ObjectRef.DeepCopy(),
	}
}

func (o *TypedObjectRef) DeepCopyInto(out *TypedObjectRef) {
	*out = *o
}

func (o *TypedObjectRef) String() string {
	return o.APIVersion + "/" + o.Kind + ":" + o.Namespace + "/" + o.Name
}
