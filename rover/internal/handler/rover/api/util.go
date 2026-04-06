// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"strings"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	rover "github.com/telekom/controlplane/rover/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func MakeName(ownerName, basePath, organization string) string {
	name := ownerName + "--" + strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
	if organization != "" {
		name = organization + "--" + name
	}
	return name
}

func toApiClient(client *rover.OAuth2ClientCredentials) *apiapi.OAuth2ClientCredentials {
	if client == nil {
		return nil
	}
	return &apiapi.OAuth2ClientCredentials{
		ClientId:     client.ClientId,
		ClientSecret: client.ClientSecret,
		ClientKey:    client.ClientKey,
	}
}

func toApiBasic(basic *rover.BasicAuthCredentials) *apiapi.BasicAuthCredentials {
	if basic == nil {
		return nil
	}
	return &apiapi.BasicAuthCredentials{
		Username: basic.Username,
		Password: basic.Password,
	}
}

// getFailoverZones converts rover failover configuration to API failover zones
// This is used for PROVIDER failover (ApiExposure), not subscriber failover
func getFailoverZones(env string, failoverCfg *rover.Failover) ([]types.ObjectRef, bool) {
	if failoverCfg == nil || len(failoverCfg.Zones) == 0 {
		return nil, false
	}

	failoverZones := make([]types.ObjectRef, len(failoverCfg.Zones))
	for i, zone := range failoverCfg.Zones {
		failoverZones[i] = types.ObjectRef{
			Name:      zone,
			Namespace: env,
		}
	}
	return failoverZones, true
}

// GetDtcEligibleZones scans all zones and returns those that are DTC-eligible
// A zone is DTC-eligible if:
// 1. It has a dtcUrl configured (zone.Spec.Gateway.DtcUrl != "")
// 2. It has Enterprise visibility (zone.Spec.Visibility == "Enterprise")
func GetDtcEligibleZones(ctx context.Context, c client.JanitorClient, env string) ([]types.ObjectRef, error) {
	log := log.FromContext(ctx)

	zoneList := &adminapi.ZoneList{}
	err := c.List(ctx, zoneList)
	if err != nil {
		return nil, err
	}

	var dtcZones []types.ObjectRef
	for _, zone := range zoneList.Items {
		// Check if zone has DTC URL configured and Enterprise visibility
		if zone.Spec.Gateway.DtcUrl != "" && zone.Spec.Visibility == adminapi.ZoneVisibilityEnterprise {
			dtcZones = append(dtcZones, types.ObjectRef{
				Name:      zone.Name,
				Namespace: env,
			})
			log.V(1).Info("Found DTC-eligible zone", "zone", zone.Name, "dtcUrl", zone.Spec.Gateway.DtcUrl)
		}
	}

	log.V(1).Info("DTC-eligible zones discovered", "count", len(dtcZones), "zones", dtcZones)
	return dtcZones, nil
}
