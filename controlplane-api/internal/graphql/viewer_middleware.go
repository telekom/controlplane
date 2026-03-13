// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entgroup "github.com/telekom/controlplane/controlplane-api/ent/group"
	entteam "github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

// ViewerFromBusinessContext is a gqlgen AroundOperations middleware that bridges
// common-server's BusinessContext (from JWT) to ent's privacy system via the Viewer.
func ViewerFromBusinessContext(client *ent.Client) graphql.OperationMiddleware {
	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		bCtx, ok := security.FromContext(ctx)
		if !ok {
			// If no business context, let privacy rules deny the query
			return next(ctx)
		}

		v := &viewer.Viewer{}

		switch bCtx.ClientType {
		case security.ClientTypeAdmin:
			v.Admin = true
		case security.ClientTypeGroup:
			// Look up all teams in the group
			sysCtx := viewer.SystemContext(ctx)
			teams, err := client.Team.Query().
				Where(entteam.HasGroupWith(entgroup.Name(bCtx.Group))).
				Select(entteam.FieldName).
				Strings(sysCtx)
			if err == nil {
				v.Teams = teams
			}
		case security.ClientTypeTeam:
			v.Teams = []string{bCtx.Team}
		}

		ctx = viewer.NewContext(ctx, v)
		return next(ctx)
	}
}
