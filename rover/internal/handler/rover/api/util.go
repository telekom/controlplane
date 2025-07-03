// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"

	"github.com/telekom/controlplane/common/pkg/types"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

func MakeName(ownerName, basePath, organization string) string {
	name := ownerName + "--" + strings.Trim(strings.ReplaceAll(basePath, "/", "-"), "-")
	if organization != "" {
		name = organization + "--" + name
	}

	return name
}

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
