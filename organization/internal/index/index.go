// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"
	"os"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	FieldSpecGroup    = "spec.group"
	FieldSpecTeamApis = "spec.teamApis"
)

func RegisterIndicesOrDie(ctx context.Context, mgr ctrl.Manager) {

	// Index the team by the group it refers to
	filterTeamGroup := func(obj client.Object) []string {
		team, ok := obj.(*organizationv1.Team)
		if !ok {
			return nil
		}
		return []string{team.Spec.Group}
	}

	filterZoneWithTeamRealmInfos := func(obj client.Object) []string {
		zone, ok := obj.(*adminv1.Zone)
		if !ok {
			return nil
		}

		if zone.Spec.TeamApis == nil {
			return []string{"false"}
		} else {
			return []string{"true"}
		}
	}

	err := mgr.GetFieldIndexer().IndexField(ctx, &organizationv1.Team{}, FieldSpecGroup, filterTeamGroup)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for team", "FieldIndex", FieldSpecGroup)
		os.Exit(1)
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &adminv1.Zone{}, FieldSpecTeamApis, filterZoneWithTeamRealmInfos)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for zone", "FieldIndex", FieldSpecTeamApis)
		os.Exit(1)
	}
}
