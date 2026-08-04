// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (r *TestResource) DeepCopyObject() runtime.Object {
	return r.DeepCopy()
}

func (r *TestResource) DeepCopy() *TestResource {
	out := &TestResource{
		TypeMeta:   r.TypeMeta,
		ObjectMeta: *r.ObjectMeta.DeepCopy(),
	}
	if r.Status.Conditions != nil {
		out.Status.Conditions = append([]metav1.Condition(nil), r.Status.Conditions...)
	}
	return out
}

func (r *TestResourceList) DeepCopyObject() runtime.Object {
	return r.DeepCopy()
}

func (r *TestResourceList) DeepCopy() *TestResourceList {
	out := &TestResourceList{
		TypeMeta: r.TypeMeta,
		ListMeta: *r.ListMeta.DeepCopy(),
	}
	for _, item := range r.Items {
		out.Items = append(out.Items, item.DeepCopy())
	}
	return out
}
