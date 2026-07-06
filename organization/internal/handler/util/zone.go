// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/organization/internal/index"
)

func GetZoneObjWithTeamInfo(ctx context.Context) (*adminv1.Zone, error) {
	zoneList := &adminv1.ZoneList{}
	clientFromContext := cclient.ClientFromContextOrDie(ctx)

	err := clientFromContext.List(ctx, zoneList, client.MatchingFields{index.FieldSpecManagedRoutes: "true"})
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	switch len(zoneList.Items) {
	case 0:
		return nil, ctrlerrors.BlockedErrorf("no zone with managed routes found")
	case 1:
		zone := zoneList.Items[0]
		if zone.Spec.ManagedRoutes == nil {
			return nil, ctrlerrors.BlockedErrorf("no zone with managed routes found")
		}
		return &zone, nil
	default:
		// multiple zones with managed routes found, this should not happen in a properly configured cluster
		return nil, ctrlerrors.BlockedErrorf("multiple zones with managed routes found")
	}
}
