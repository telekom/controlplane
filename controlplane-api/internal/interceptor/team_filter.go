// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package interceptor

import (
	"context"
	"fmt"

	"entgo.io/ent"

	entgen "github.com/telekom/controlplane/controlplane-api/ent"
	"github.com/telekom/controlplane/controlplane-api/ent/apiexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/apisubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/application"
	"github.com/telekom/controlplane/controlplane-api/ent/approval"
	"github.com/telekom/controlplane/controlplane-api/ent/approvalrequest"
	"github.com/telekom/controlplane/controlplane-api/ent/eventexposure"
	"github.com/telekom/controlplane/controlplane-api/ent/eventsubscription"
	"github.com/telekom/controlplane/controlplane-api/ent/member"
	"github.com/telekom/controlplane/controlplane-api/ent/privacy"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

// TeamFilterInterceptor returns an interceptor that filters queries based on team membership.
func TeamFilterInterceptor() ent.Interceptor {
	return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			// If privacy.Allow set in the context? If so, skip team filtering (e.g. for system resolvers)
			if _, ok := privacy.DecisionFromContext(ctx); ok {
				return next.Query(ctx, query)
			}

			v := viewer.FromContext(ctx)
			if v == nil || v.Admin {
				return next.Query(ctx, query)
			}
			if len(v.Teams) == 0 {
				// Let privacy rules handle the denial
				return next.Query(ctx, query)
			}

			teams := v.Teams

			switch q := query.(type) {
			case *entgen.TeamQuery:
				q.Where(team.NameIn(teams...))

			case *entgen.ApplicationQuery:
				q.Where(application.HasOwnerTeamWith(team.NameIn(teams...)))

			case *entgen.ApiExposureQuery:
				q.Where(apiexposure.HasOwnerWith(
					application.HasOwnerTeamWith(team.NameIn(teams...)),
				))

			case *entgen.ApiSubscriptionQuery:
				q.Where(apisubscription.HasOwnerWith(
					application.HasOwnerTeamWith(team.NameIn(teams...)),
				))

			case *entgen.ApprovalQuery:
				q.Where(approval.Or(
					approval.HasAPISubscriptionWith(
						apisubscription.HasOwnerWith(
							application.HasOwnerTeamWith(team.NameIn(teams...)),
						),
					),
					approval.HasAPISubscriptionWith(
						apisubscription.HasTargetWith(
							apiexposure.HasOwnerWith(
								application.HasOwnerTeamWith(team.NameIn(teams...)),
							),
						),
					),
					approval.HasEventSubscriptionWith(
						eventsubscription.HasOwnerWith(
							application.HasOwnerTeamWith(team.NameIn(teams...)),
						),
					),
					approval.HasEventSubscriptionWith(
						eventsubscription.HasTargetWith(
							eventexposure.HasOwnerWith(
								application.HasOwnerTeamWith(team.NameIn(teams...)),
							),
						),
					),
				))

			case *entgen.ApprovalRequestQuery:
				q.Where(approvalrequest.Or(
					approvalrequest.HasAPISubscriptionWith(
						apisubscription.HasOwnerWith(
							application.HasOwnerTeamWith(team.NameIn(teams...)),
						),
					),
					approvalrequest.HasAPISubscriptionWith(
						apisubscription.HasTargetWith(
							apiexposure.HasOwnerWith(
								application.HasOwnerTeamWith(team.NameIn(teams...)),
							),
						),
					),
					approvalrequest.HasEventSubscriptionWith(
						eventsubscription.HasOwnerWith(
							application.HasOwnerTeamWith(team.NameIn(teams...)),
						),
					),
					approvalrequest.HasEventSubscriptionWith(
						eventsubscription.HasTargetWith(
							eventexposure.HasOwnerWith(
								application.HasOwnerTeamWith(team.NameIn(teams...)),
							),
						),
					),
				))

			case *entgen.MemberQuery:
				q.Where(member.HasTeamWith(team.NameIn(teams...)))

			case *entgen.EventExposureQuery:
				q.Where(eventexposure.HasOwnerWith(
					application.HasOwnerTeamWith(team.NameIn(teams...)),
				))

			case *entgen.EventSubscriptionQuery:
				q.Where(eventsubscription.HasOwnerWith(
					application.HasOwnerTeamWith(team.NameIn(teams...)),
				))

			case *entgen.GroupQuery, *entgen.ZoneQuery, *entgen.APIQuery, *entgen.EventTypeQuery:
				// No team filtering for public/catalogue entities
			default:
				return nil, fmt.Errorf("team filter: unsupported query type %T", query)
			}

			return next.Query(ctx, query)
		})
	})
}
