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
	"github.com/telekom/controlplane/controlplane-api/ent/member"
	"github.com/telekom/controlplane/controlplane-api/ent/team"
	"github.com/telekom/controlplane/controlplane-api/internal/viewer"
)

// TeamFilterInterceptor returns an interceptor that filters queries based on team membership.
func TeamFilterInterceptor() ent.Interceptor {
	return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
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
				))

			case *entgen.MemberQuery:
				q.Where(member.HasTeamWith(team.NameIn(teams...)))

			case *entgen.GroupQuery, *entgen.ZoneQuery:
				// No team filtering for public entities

			default:
				return nil, fmt.Errorf("team filter: unsupported query type %T", query)
			}

			return next.Query(ctx, query)
		})
	})
}
