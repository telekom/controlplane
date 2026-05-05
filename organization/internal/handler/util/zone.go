// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/organization/internal/index"
)

func GetZoneObjWithTeamInfo(ctx context.Context) (*adminv1.Zone, error) {
	var teamApiZone *adminv1.Zone = nil
	zoneList := &adminv1.ZoneList{}
	clientFromContext := cclient.ClientFromContextOrDie(ctx)

	err := clientFromContext.List(ctx, zoneList, client.MatchingFields{index.FieldSpecTeamApis: "true"})
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	for _, zone := range zoneList.GetItems() {
		z, ok := zone.(*adminv1.Zone)
		if !ok {
			continue
		}
		if z.Spec.TeamApis != nil {
			teamApiZone = z.DeepCopy()
			break
		}
	}

	if teamApiZone == nil {
		return nil, errors.New("found no zone with team apis")
	}

	return teamApiZone, nil
}
