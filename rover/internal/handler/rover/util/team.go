// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"
)

// FindTeam finds the team for the given owner identified by the resource namespace.
func FindTeam(ctx context.Context, c client.JanitorClient, namespace string) (*organizationv1.Team, error) {

	// find owners team with help of resource namespace <environment>--<group>--<team>
	roverNamespaceParts := strings.Split(namespace, "--")

	if len(roverNamespaceParts) != 3 {
		return nil, errors.New("invalid rover resource namespace")
	}

	team := &organizationv1.Team{}
	teamRef := types.ObjectRef{
		Name:      roverNamespaceParts[1] + "--" + roverNamespaceParts[2],
		Namespace: roverNamespaceParts[0],
	}

	err := c.Get(ctx, teamRef.K8s(), team)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to get team %s", teamRef.Name)
	}

	return team, nil
}
