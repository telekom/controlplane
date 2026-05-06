// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	"github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// MapResponse maps an ApiSubscription CRD to an ApiSubscriptionResponse,
// resolving cross-resource references (Zone, Application, Approval) from stores.
func MapResponse(ctx context.Context, in *apiv1.ApiSubscription, stores *sstore.Stores) api.ApiSubscriptionResponse {
	resp := api.ApiSubscriptionResponse{
		Name:     mapper.MakeResourceName(in),
		BasePath: in.Spec.ApiBasePath,
		Zone:     in.Spec.Zone.Name,
		Status:   status.MapStatus(in.GetConditions(), in.GetGeneration()),
	}

	mapSecurity(in, &resp)
	mapFailover(ctx, in, &resp, stores.ZoneStore)
	mapGatewayUrl(ctx, in, &resp, stores.ZoneStore)
	mapTeamAndApplication(ctx, in, &resp, stores.ApplicationStore)
	mapApproval(ctx, in, &resp, stores.ApprovalStore)

	return resp
}

func mapSecurity(in *apiv1.ApiSubscription, out *api.ApiSubscriptionResponse) {
	if in.Spec.Security == nil || in.Spec.Security.M2M == nil {
		return
	}

	m2m := in.Spec.Security.M2M

	if m2m.Basic != nil {
		basicAuth := api.BasicAuth{
			Username: m2m.Basic.Username,
			Password: m2m.Basic.Password,
		}
		out.Security = api.SubscriberSecurity{}
		out.Security.FromBasicAuth(basicAuth) //nolint:errcheck,gosec // union setter only fails on JSON marshal of simple struct
		return
	}

	if m2m.Client != nil {
		oauth2 := api.OAuth2{
			ClientId:     m2m.Client.ClientId,
			ClientSecret: m2m.Client.ClientSecret,
		}

		if len(m2m.Scopes) > 0 {
			oauth2.Scopes = m2m.Scopes
		}

		out.Security = api.SubscriberSecurity{}
		out.Security.FromOAuth2(oauth2) //nolint:errcheck,gosec // union setter only fails on JSON marshal of simple struct
		return
	}

	// Scopes only.
	if len(m2m.Scopes) > 0 {
		oauth2 := api.OAuth2{
			Scopes: m2m.Scopes,
		}
		out.Security = api.SubscriberSecurity{}
		out.Security.FromOAuth2(oauth2) //nolint:errcheck,gosec // union setter only fails on JSON marshal of simple struct
	}
}

func mapFailover(ctx context.Context, in *apiv1.ApiSubscription, out *api.ApiSubscriptionResponse, zoneStore store.ObjectStore[*adminv1.Zone]) {
	if in.Spec.Traffic.Failover == nil || len(in.Spec.Traffic.Failover.Zones) == 0 {
		return
	}

	failover := make(api.SubscriptionFailover, 0, len(in.Spec.Traffic.Failover.Zones))
	for _, zoneRef := range in.Spec.Traffic.Failover.Zones {
		entry := struct {
			GatewayUrl string `json:"gatewayUrl,omitempty,omitzero"`
			Zone       string `json:"zone,omitempty,omitzero"`
		}{
			Zone: zoneRef.Name,
		}

		// Resolve gateway URL for the failover zone.
		zone, err := zoneStore.Get(ctx, zoneRef.Namespace, zoneRef.Name)
		if err == nil && zone != nil {
			entry.GatewayUrl = joinURL(zone.Status.Links.Url, in.Spec.ApiBasePath)
		}

		failover = append(failover, entry)
	}
	out.Failover = failover
}

func mapGatewayUrl(ctx context.Context, in *apiv1.ApiSubscription, out *api.ApiSubscriptionResponse, zoneStore store.ObjectStore[*adminv1.Zone]) {
	zone, err := zoneStore.Get(ctx, in.Spec.Zone.Namespace, in.Spec.Zone.Name)
	if err != nil || zone == nil {
		return
	}
	out.GatewayUrl = joinURL(zone.Status.Links.Url, in.Spec.ApiBasePath)
}

func mapTeamAndApplication(ctx context.Context, in *apiv1.ApiSubscription, out *api.ApiSubscriptionResponse, appStore store.ObjectStore[*applicationv1.Application]) {
	appRef := in.Spec.Requestor.Application
	app, err := appStore.Get(ctx, appRef.Namespace, appRef.Name)
	if err != nil || app == nil {
		// Best effort — fill what we can from the namespace.
		nsInfo := mapper.ParseNamespace(in.GetNamespace())
		out.Team = api.TeamRef{
			Hub:  nsInfo.Group,
			Name: nsInfo.Team,
		}
		out.Application = api.ApplicationRef{
			Name: appRef.Name,
		}
		return
	}

	nsInfo := mapper.ParseNamespace(app.GetNamespace())
	out.Team = api.TeamRef{
		Hub:   nsInfo.Group,
		Name:  nsInfo.Team,
		Email: openapi_types.Email(app.Spec.TeamEmail),
	}
	out.Application = api.ApplicationRef{
		Name: app.GetName(),
	}
}

func mapApproval(ctx context.Context, in *apiv1.ApiSubscription, out *api.ApiSubscriptionResponse, approvalStore store.ObjectStore[*approvalv1.Approval]) {
	if in.Status.Approval == nil {
		return
	}

	approval, err := approvalStore.Get(ctx, in.Status.Approval.Namespace, in.Status.Approval.Name)
	if err != nil || approval == nil {
		return
	}

	out.Approval = api.Approval{
		Status: string(approval.Spec.State),
	}

	// Add the latest decision details if available.
	if len(approval.Spec.Decisions) > 0 {
		latest := approval.Spec.Decisions[len(approval.Spec.Decisions)-1]
		out.Approval.Decider = latest.Email
		out.Approval.Comment = latest.Comment
	}
}

// joinURL concatenates a base URL and a path, ensuring exactly one "/" between them.
// TODO: use url.JoinPath(base, path)
func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}
