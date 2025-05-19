// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Equals is *NOT* a deep-equals.
// It will compare the name, namespace and GVK of the two objects.
// If both objects are nil, it will return true.
func Equals(obj1, obj2 client.Object) bool {
	if obj1 == nil && obj2 == nil {
		return true
	}
	if (obj1 == nil) != (obj2 == nil) {
		return false
	}
	return obj1.GetName() == obj2.GetName() &&
		obj1.GetNamespace() == obj2.GetNamespace() &&
		obj1.GetObjectKind().GroupVersionKind() == obj2.GetObjectKind().GroupVersionKind()
}
