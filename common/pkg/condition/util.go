// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"fmt"

	"github.com/telekom/controlplane/common/pkg/types"

	"k8s.io/apimachinery/pkg/api/meta"
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
