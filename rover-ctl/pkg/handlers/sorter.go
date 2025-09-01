// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"sort"

	"github.com/telekom/controlplane/rover-ctl/pkg/log"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

// Sort sorts the given objects based on their handler priority
// Handlers with higher priority (lower numerical value) will come first
// If a handler is not found for an object, it will not be sorted
// The sorting is stable, meaning objects with the same priority will maintain their relative order
func Sort(objects []types.Object) []types.Object {
	// Sort objects by their handler priority
	sorted := make([]types.Object, len(objects))
	copy(sorted, objects)

	sort.Slice(sorted, func(i, j int) bool {
		handlerI, errI := GetHandler(sorted[i].GetKind(), sorted[i].GetApiVersion())
		handlerJ, errJ := GetHandler(sorted[j].GetKind(), sorted[j].GetApiVersion())

		if errI != nil || errJ != nil {
			log.L().V(1).Info("Handler not found for sorting", "objectI", sorted[i], "objectJ", sorted[j], "errorI", errI, "errorJ", errJ)
			return false // If any handler is not found, do not sort
		}

		return handlerI.Priority() < handlerJ.Priority()
	})

	return sorted
}
