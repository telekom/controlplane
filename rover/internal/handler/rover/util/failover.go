// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

func GetDtcFailoverZones(ctx context.Context, owner *rover.Rover, env string) ([]types.ObjectRef, error) {
	if !owner.Spec.FailoverEnabled {
		return nil, nil
	}

	c := client.ClientFromContextOrDie(ctx)

	// List all zones in the environment
	zoneList := &adminv1.ZoneList{}
	err := c.List(ctx, zoneList)
	if err != nil {
		return nil, err
	}

	// Filter zones that are DTC-eligible:
	// 1. Have DtcUrl configured
	// 2. Have Enterprise visibility
	// 3. Exclude the rover's own zone (parent zone)
	var failoverZones []types.ObjectRef
	for _, zone := range zoneList.Items {
		// Skip the rover's own zone
		if zone.Name == owner.Spec.Zone {
			continue
		}

		if zone.Spec.Gateway.DtcUrl != nil && *zone.Spec.Gateway.DtcUrl != "" &&
			zone.Spec.Visibility == adminv1.ZoneVisibilityEnterprise {
			failoverZones = append(failoverZones, types.ObjectRef{
				Name:      zone.Name,
				Namespace: zone.Namespace,
			})
		}
	}

	return failoverZones, nil
}
