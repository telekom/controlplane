// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	"context"
	"encoding/json"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	"github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

// MapResponse maps an EventExposure CRD to an EventExposureResponse,
// resolving cross-resource references (Application) from stores.
func MapResponse(ctx context.Context, in *eventv1.EventExposure, stores *sstore.Stores) api.EventExposureResponse {
	resp := api.EventExposureResponse{
		Name:       mapper.MakeResourceName(in),
		EventType:  in.Spec.EventType,
		Visibility: toAPIVisibility(in.Spec.Visibility),
		Approval: api.EventApproval{
			Strategy:     toAPIApprovalStrategy(in.Spec.Approval.Strategy),
			TrustedTeams: in.Spec.Approval.TrustedTeams,
		},
		Zone:                   in.Spec.Zone.Name,
		Active:                 in.Status.Active,
		CallbackURL:            in.Status.CallbackURL,
		SseUrls:                in.Status.SseURLs,
		AdditionalPublisherIds: in.Spec.AdditionalPublisherIds,
		Status:                 status.MapStatus(in.GetConditions(), in.GetGeneration()),
	}

	mapScopes(in, &resp)
	mapTeamAndApplication(ctx, in, &resp, stores.ApplicationStore)

	return resp
}

func mapScopes(in *eventv1.EventExposure, out *api.EventExposureResponse) {
	if len(in.Spec.Scopes) == 0 {
		return
	}
	scopes := make([]api.EventScope, len(in.Spec.Scopes))
	for i, s := range in.Spec.Scopes {
		scopes[i] = api.EventScope{
			Name:    s.Name,
			Trigger: mapEventTrigger(s.Trigger),
		}
	}
	out.Scopes = scopes
}

func mapTeamAndApplication(ctx context.Context, in *eventv1.EventExposure, out *api.EventExposureResponse, appStore store.ObjectStore[*applicationv1.Application]) {
	appName := in.Spec.Provider.Name
	appNamespace := in.Spec.Provider.Namespace
	app, err := appStore.Get(ctx, appNamespace, appName)
	if err != nil || app == nil {
		// Best effort — fill what we can from the namespace.
		nsInfo := mapper.ParseNamespace(in.GetNamespace())
		out.Team = api.TeamRef{
			Hub:  nsInfo.Group,
			Name: nsInfo.Team,
		}
		out.Application = api.ApplicationRef{
			Name: appName,
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

func mapEventTrigger(in eventv1.EventTrigger) api.EventTrigger {
	out := api.EventTrigger{}
	if in.ResponseFilter != nil {
		out.ResponseFilter = api.ResponseFilter{
			Paths: in.ResponseFilter.Paths,
			Mode:  api.ResponseFilterMode(in.ResponseFilter.Mode),
		}
	}
	if in.SelectionFilter != nil {
		out.SelectionFilter = api.SelectionFilter{
			Attributes: in.SelectionFilter.Attributes,
		}
		if in.SelectionFilter.Expression != nil && in.SelectionFilter.Expression.Raw != nil {
			// The Expression is stored as apiextensionsv1.JSON (raw bytes).
			// Unmarshal into map[string]interface{} for the API response.
			var expr map[string]interface{}
			if err := json.Unmarshal(in.SelectionFilter.Expression.Raw, &expr); err == nil {
				out.SelectionFilter.Expression = expr
			}
		}
	}
	return out
}

func toAPIVisibility(v eventv1.Visibility) api.EventVisibility {
	switch v {
	case eventv1.VisibilityWorld:
		return api.World
	case eventv1.VisibilityZone:
		return api.Zone
	case eventv1.VisibilityEnterprise:
		return api.Enterprise
	default:
		return api.EventVisibility(strings.ToUpper(string(v)))
	}
}

func toAPIApprovalStrategy(s eventv1.ApprovalStrategy) api.EventApprovalStrategy {
	switch s {
	case eventv1.ApprovalStrategyAuto:
		return api.Auto
	case eventv1.ApprovalStrategySimple:
		return api.Simple
	case eventv1.ApprovalStrategyFourEyes:
		return api.FourEyes
	default:
		return api.EventApprovalStrategy(strings.ToUpper(string(s)))
	}
}
