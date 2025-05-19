// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// **Note:** Please have a look at [api-conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status)
// for the conventions around the `spec` and `status` fields.
//
// Espectially regarding conditions, see [here](https://github.com/kubernetes/apimachinery/blob/release-1.23/pkg/apis/meta/v1/types.go#L1448)
type Object interface {
	client.Object
	GetConditions() []metav1.Condition
	SetCondition(metav1.Condition) bool
}

type ObjectList interface {
	client.ObjectList
	GetItems() []Object
}
