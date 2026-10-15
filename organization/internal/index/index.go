// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package index

import (
	"context"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

const (
	FieldSpecGroup         = "spec.group"
	FieldSpecManagedRoutes = "spec.managedRoutes"
	FieldStatusNamespace   = "status.namespace"
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

	// Index the team by the namespace it provisions (status.namespace)
	filterTeamStatusNamespace := func(obj client.Object) []string {
		team, ok := obj.(*organizationv1.Team)
		if !ok {
			return nil
		}
		return []string{team.Status.Namespace}
	}

	filterZoneWithTeamRealmInfos := func(obj client.Object) []string {
		zone, ok := obj.(*adminv1.Zone)
		if !ok {
			return nil
		}

		if zone.Spec.ManagedRoutes != nil {
			for _, r := range zone.Spec.ManagedRoutes.Routes {
				if r.Type == adminv1.ManagedRouteTypeTeamAPI {
					return []string{"true"}
				}
			}
		}
		return []string{"false"}
	}

	err := mgr.GetFieldIndexer().IndexField(ctx, &organizationv1.Team{}, FieldSpecGroup, filterTeamGroup)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for team", "FieldIndex", FieldSpecGroup)
		os.Exit(1)
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &organizationv1.Team{}, FieldStatusNamespace, filterTeamStatusNamespace)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for team", "FieldIndex", FieldStatusNamespace)
		os.Exit(1)
	}

	err = mgr.GetFieldIndexer().IndexField(ctx, &adminv1.Zone{}, FieldSpecManagedRoutes, filterZoneWithTeamRealmInfos)
	if err != nil {
		ctrl.Log.Error(err, "unable to create fieldIndex for zone", "FieldIndex", FieldSpecManagedRoutes)
		os.Exit(1)
	}
}
