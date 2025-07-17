// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"slices"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
)

type Traffic struct {
	// Failover defines the failover configuration for the API exposure.
	// +kubebuilder:validation:Optional
	Failover *Failover `json:"failover,omitempty"`
}
type Failover struct {
	// Zone is the zone to which the traffic should be failed over in case of an error.
	// +kubebuilder:validation:Required
	Zones []ctypes.ObjectRef `json:"zone"`
}

func (t *Traffic) HasFailover() bool {
	return t.Failover != nil && len(t.Failover.Zones) > 0
}

func (f Failover) ContainsZone(zone ctypes.ObjectRef) bool {
	return slices.ContainsFunc(f.Zones, func(z ctypes.ObjectRef) bool {
		return z.Equals(&zone)
	})
}
