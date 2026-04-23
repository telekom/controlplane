// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"context"
	"encoding/json"

	openapi_types "github.com/oapi-codegen/runtime/types"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"

	"github.com/telekom/controlplane/discovery-server/internal/api"
	"github.com/telekom/controlplane/discovery-server/internal/mapper"
	"github.com/telekom/controlplane/discovery-server/internal/mapper/status"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
)

// MapResponse maps an EventSubscription CRD to an EventSubscriptionResponse,
// resolving cross-resource references (Application, Approval) from stores.
func MapResponse(ctx context.Context, in *eventv1.EventSubscription, stores *sstore.Stores) api.EventSubscriptionResponse {
	resp := api.EventSubscriptionResponse{
		Name:           mapper.MakeResourceName(in),
		EventType:      in.Spec.EventType,
		Zone:           in.Spec.Zone.Name,
		Scopes:         in.Spec.Scopes,
		SubscriptionId: in.Status.SubscriptionId,
		Url:            in.Status.URL,
		Status:         status.MapStatus(in.GetConditions(), in.GetGeneration()),
	}

	mapDelivery(in, &resp)
	mapTrigger(in, &resp)
	mapTeamAndApplication(ctx, in, &resp, stores.ApplicationStore)
	mapApproval(ctx, in, &resp, stores.ApprovalStore)

	return resp
}

func mapDelivery(in *eventv1.EventSubscription, out *api.EventSubscriptionResponse) {
	d := in.Spec.Delivery
	out.Delivery = api.Delivery{
		Type:                 api.DeliveryType(d.Type),
		Payload:              api.PayloadType(d.Payload),
		Callback:             d.Callback,
		EventRetentionTime:   d.EventRetentionTime,
		CircuitBreakerOptOut: d.CircuitBreakerOptOut,
		RetryableStatusCodes: d.RetryableStatusCodes,
		EnforceGetHttpRequestMethodForHealthCheck: d.EnforceGetHttpRequestMethodForHealthCheck,
	}
	if d.RedeliveriesPerSecond != nil {
		out.Delivery.RedeliveriesPerSecond = *d.RedeliveriesPerSecond
	}
}

func mapTrigger(in *eventv1.EventSubscription, out *api.EventSubscriptionResponse) {
	if in.Spec.Trigger == nil {
		return
	}
	out.Trigger = mapEventTrigger(*in.Spec.Trigger)
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
			var expr map[string]interface{}
			if err := json.Unmarshal(in.SelectionFilter.Expression.Raw, &expr); err == nil {
				out.SelectionFilter.Expression = expr
			}
		}
	}
	return out
}

func mapTeamAndApplication(ctx context.Context, in *eventv1.EventSubscription, out *api.EventSubscriptionResponse, appStore store.ObjectStore[*applicationv1.Application]) {
	appName := in.Spec.Requestor.Name
	appNamespace := in.Spec.Requestor.Namespace
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

func mapApproval(ctx context.Context, in *eventv1.EventSubscription, out *api.EventSubscriptionResponse, approvalStore store.ObjectStore[*approvalv1.Approval]) {
	if in.Status.Approval == nil {
		return
	}

	approval, err := approvalStore.Get(ctx, in.Status.Approval.Namespace, in.Status.Approval.Name)
	if err != nil || approval == nil {
		return
	}

	out.Approval = api.EventSubscriptionApproval{
		Status: string(approval.Spec.State),
	}

	// Add the latest decision details if available.
	if len(approval.Spec.Decisions) > 0 {
		latest := approval.Spec.Decisions[len(approval.Spec.Decisions)-1]
		out.Approval.Decider = latest.Email
		out.Approval.Comment = latest.Comment
	}
}
