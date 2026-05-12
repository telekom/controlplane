// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/go-logr/logr"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/controlplane-api/ent"
	entgroup "github.com/telekom/controlplane/controlplane-api/ent/group"
	"github.com/telekom/controlplane/controlplane-api/ent/member"
	entteam "github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

// ViewerFromBusinessContext is a gqlgen AroundOperations middleware that bridges
// common-server's BusinessContext (from JWT) to ent's privacy system via the Viewer.
// When securityEnabled is false and no BusinessContext is present, an admin viewer
// is injected so that the GraphQL playground works without authentication.
func ViewerFromBusinessContext(client *ent.Client, securityEnabled bool) graphql.OperationMiddleware {

	return func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		v := &viewer.Viewer{}
		logger := logr.FromContextOrDiscard(ctx).WithName("viewer-middleware")

		bCtx, ok := security.FromContext(ctx)
		if !ok && securityEnabled {
			// If security is enabled but no BusinessContext is found, return an error response.
			// This should not happen in practice since the security middleware should reject unauthenticated requests,
			// but we add this check for extra safety.
			return graphql.OneShot(graphql.ErrorResponse(ctx, "no business context found"))
		}

		if ok {
			switch bCtx.ClientType {
			case security.ClientTypeAdmin:
				v.Admin = true

			case security.ClientTypeGroup:
				v.Group = bCtx.Group
				// SystemContext: No Viewer exists yet — we need to query teams for
				// this group in order to build the Viewer that privacy rules will use.
				sysCtx := viewer.SystemContext(ctx)
				teams, err := client.Team.Query().
					Where(entteam.HasGroupWith(entgroup.Name(bCtx.Group))).
					Select(entteam.FieldName).
					Strings(sysCtx)
				if err != nil {
					logger.Error(err, "failed to resolve teams for group", "group", bCtx.Group)
					return graphql.OneShot(graphql.ErrorResponse(ctx, "failed to resolve teams for group"))

				} else {
					v.Teams = teams
				}

			case security.ClientTypeTeam:
				v.Teams = []string{bCtx.Team}
			}

		} else if !securityEnabled {
			logger.Info("No business context found, but security is disabled - injecting admin viewer for playground access")
			v.Admin = true

		}

		// Populate forwarded user identity if present in context.
		// Only accept forwarded user identity from admin-scoped clients (i.e. the BFF),
		// preventing non-admin services from injecting identity headers to escalate privileges.
		// TODO: X-Forwarded-User-Is-Admin should be verified against an authoritative
		// source (e.g. DB role lookup) rather than trusted from the header. Additionally,
		// restrict forwarded identity acceptance to a specific BFF client ID allowlist.

		if fu, hasFU := viewer.ForwardedUserFromContext(ctx); hasFU {
			if !v.Admin {
				logger.Info("Ignoring forwarded user identity from non-admin client",
					"userName", fu.Name, "userEmail", fu.Email)

			} else {
				logger.Info("Processing forwarded user identity",
					"userName", fu.Name, "userEmail", fu.Email, "claimedAdmin", fu.IsAdmin)

				v.UserName = fu.Name
				v.UserEmail = fu.Email

				// When a user email is present (BFF request on behalf of a user),
				// scope access based on the user's actual team memberships.
				if fu.Email != "" {
					// SystemContext: No Viewer exists yet — we need to resolve the user's
					// team memberships to build the Viewer that privacy rules will use.
					sysCtx := viewer.SystemContext(ctx)
					userTeams, err := client.Team.Query().
						Where(entteam.HasMembersWith(member.EmailEQ(fu.Email))).
						Select(entteam.FieldName).
						Strings(sysCtx)
					if err != nil {
						logger.Error(err, "failed to resolve teams for user", "userEmail", fu.Email)
						return graphql.OneShot(graphql.ErrorResponse(ctx, "failed to resolve teams for user"))
					}

					// Allow X-Forwarded-User-Is-Admin header to override admin status when email is present, for flexibility in BFF behavior.
					v.Admin = fu.IsAdmin
					v.Teams = userTeams
				}
			}
		}

		ctx = viewer.NewContext(ctx, v)
		return next(ctx)
	}
}
