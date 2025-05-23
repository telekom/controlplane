// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package group

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
	"github.com/telekom/controlplane/organization/internal/index"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetGroupByName(ctx context.Context, groupName string) (*organizationv1.Group, error) {
	group := &organizationv1.Group{}
	k8sClient := cclient.ClientFromContextOrDie(ctx)
	env := contextutil.EnvFromContextOrDie(ctx)

	err := k8sClient.Get(ctx, types.NamespacedName{Name: groupName, Namespace: env}, group)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to get group '%s' in namespace (env) '%s'", groupName, env))
	}
	return group, nil
}

func GetTeamsForGroup(ctx context.Context, groupObj *organizationv1.Group) (*organizationv1.TeamList, error) {
	clientFromContext := cclient.ClientFromContextOrDie(ctx)

	teamList := &organizationv1.TeamList{}
	err := clientFromContext.List(ctx, teamList, client.MatchingFields{index.FieldSpecGroup: groupObj.GetName()})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list teams")
	}

	return teamList, nil
}
