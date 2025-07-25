// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"strings"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
)

// IsRequesterFromTrustedTeam checks if the ApiSubscription requester is from a trusted team
func IsRequesterFromTrustedTeam(ctx context.Context, apiSub *apiapi.ApiSubscription, trustedTeams []types.ObjectRef) (bool, error) {
	client := cclient.ClientFromContextOrDie(ctx)

	var application applicationv1.Application
	err := client.Get(ctx, apiSub.Spec.Requestor.Application.K8s(), &application)
	if err != nil {
		return false, err
	}

	requesterTeamName := strings.ToLower(application.Spec.Team)

	for _, trustedTeamRef := range trustedTeams {
		if requesterTeamName == strings.ToLower(trustedTeamRef.Name) {
			return true, nil
		}
	}

	return false, nil
}
