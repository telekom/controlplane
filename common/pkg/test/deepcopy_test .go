// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package test

import "k8s.io/apimachinery/pkg/runtime"

func (r *TestResource) DeepCopyObject() runtime.Object {
	return r.DeepCopy()
}

func (r *TestResource) DeepCopy() *TestResource {
	return &TestResource{
		TypeMeta:   r.TypeMeta,
		ObjectMeta: *r.ObjectMeta.DeepCopy(),
	}
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
