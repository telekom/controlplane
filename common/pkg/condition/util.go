// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
)

// EnsureReady returns an error if the provided obj is not ready
// The error message is already formatted and should be used as is
func EnsureReady(obj types.Object) error {
	ready := meta.IsStatusConditionTrue(obj.GetConditions(), ConditionTypeReady)
	if !ready {
		return fmt.Errorf("%s '%s/%s' is not ready", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName())
	}
	return nil
}

// IsReady returns true if the provided obj is ready, false otherwise
// An object is considered ready if it has a condition of type Ready with status True and the observed generation matches the actual generation of the object
func IsReady(obj types.Object) bool {
	readyCond := meta.FindStatusCondition(obj.GetConditions(), ConditionTypeReady)
	if readyCond == nil {
		return false
	}
	actualGen := obj.GetGeneration()
	observedGen := readyCond.ObservedGeneration
	return readyCond.Status == metav1.ConditionTrue && observedGen == actualGen
}
