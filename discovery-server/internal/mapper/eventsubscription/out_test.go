// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestMapResponse_ResolvedReferences(t *testing.T) {
	t.Parallel()

	redelivery := 7

	in := &eventv1.EventSubscription{}
	in.Name = "my-app--de-telekom-eni-quickstart-v1"
	in.Namespace = "poc--eni--hyperion"
	in.Spec.EventType = "de.telekom.eni.quickstart.v1"
	in.Spec.Zone = ctypes.ObjectRef{Name: "dataplane1"}
	in.Spec.Requestor = ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}}
	in.Spec.Delivery = eventv1.Delivery{
		Type:                  eventv1.DeliveryTypeCallback,
		Payload:               eventv1.PayloadTypeData,
		Callback:              "https://my-app.example/events",
		RedeliveriesPerSecond: &redelivery,
	}
	in.Spec.Trigger = &eventv1.EventTrigger{SelectionFilter: &eventv1.SelectionFilter{Expression: &apiextensionsv1.JSON{Raw: []byte(`{"foo":"bar"}`)}}}
	in.Status.SubscriptionId = "sub-1"
	in.Status.URL = "https://sse.example"
	in.Status.Approval = &ctypes.ObjectRef{Name: "approval", Namespace: "poc--eni--hyperion"}

	app := &applicationv1.Application{}
	app.Name = "my-app"
	app.Namespace = "poc--eni--hyperion"
	app.Spec.TeamEmail = "hyperion@telekom.de"

	approval := &approvalv1.Approval{}
	approval.Spec.State = approvalv1.ApprovalStateGranted
	approval.Spec.Decisions = []approvalv1.Decision{{Email: "admin@telekom.de", Comment: "approved"}}

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return(app, nil)

	approvalStore := csmocks.NewMockObjectStore[*approvalv1.Approval](t)
	approvalStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "approval").Return(approval, nil)

	resp := MapResponse(context.Background(), in, &sstore.Stores{ApplicationStore: appStore, ApprovalStore: approvalStore})

	if resp.Team.Hub != "eni" || resp.Team.Name != "hyperion" {
		t.Fatalf("unexpected team mapping: %#v", resp.Team)
	}
	if resp.Delivery.RedeliveriesPerSecond != 7 {
		t.Fatalf("unexpected delivery mapping: %#v", resp.Delivery)
	}
	if resp.Trigger.SelectionFilter.Expression == nil {
		t.Fatalf("expected mapped trigger expression, got %#v", resp.Trigger)
	}
	if resp.Approval.Decider != "admin@telekom.de" || resp.Approval.Comment != "approved" {
		t.Fatalf("unexpected approval mapping: %#v", resp.Approval)
	}
}

func TestMapResponse_FallbackApplication(t *testing.T) {
	t.Parallel()

	in := &eventv1.EventSubscription{}
	in.Name = "my-app--de-telekom-eni-quickstart-v1"
	in.Namespace = "poc--eni--hyperion"
	in.Spec.Requestor = ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}}

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return((*applicationv1.Application)(nil), context.Canceled)

	approvalStore := csmocks.NewMockObjectStore[*approvalv1.Approval](t)

	resp := MapResponse(context.Background(), in, &sstore.Stores{ApplicationStore: appStore, ApprovalStore: approvalStore})
	if resp.Team.Hub != "eni" || resp.Team.Name != "hyperion" {
		t.Fatalf("unexpected fallback team mapping: %#v", resp.Team)
	}
}

func TestMapEventTrigger_InvalidExpression(t *testing.T) {
	t.Parallel()

	in := eventv1.EventTrigger{SelectionFilter: &eventv1.SelectionFilter{Expression: &apiextensionsv1.JSON{Raw: []byte(`not-json`)}}}
	out := mapEventTrigger(in)
	if out.SelectionFilter.Expression != nil {
		t.Fatalf("expected nil expression for invalid json, got %#v", out.SelectionFilter.Expression)
	}
}

func TestMapEventTrigger_ResponseFilter(t *testing.T) {
	t.Parallel()

	in := eventv1.EventTrigger{
		ResponseFilter: &eventv1.ResponseFilter{
			Paths: []string{"$.data", "$.meta"},
			Mode:  eventv1.ResponseFilterModeExclude,
		},
	}
	out := mapEventTrigger(in)
	if out.ResponseFilter.Mode != "Exclude" || len(out.ResponseFilter.Paths) != 2 {
		t.Fatalf("unexpected response filter mapping: %#v", out.ResponseFilter)
	}
}

func TestMapResponse_ApprovalLookupFails(t *testing.T) {
	t.Parallel()

	in := &eventv1.EventSubscription{}
	in.Name = "my-app--de-telekom-eni-quickstart-v1"
	in.Namespace = "poc--eni--hyperion"
	in.Spec.Requestor = ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}}
	in.Status.Approval = &ctypes.ObjectRef{Name: "approval", Namespace: "poc--eni--hyperion"}

	app := &applicationv1.Application{}
	app.Name = "my-app"
	app.Namespace = "poc--eni--hyperion"

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return(app, nil)

	approvalStore := csmocks.NewMockObjectStore[*approvalv1.Approval](t)
	approvalStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "approval").Return((*approvalv1.Approval)(nil), context.Canceled)

	resp := MapResponse(context.Background(), in, &sstore.Stores{ApplicationStore: appStore, ApprovalStore: approvalStore})
	if resp.Approval.Status != "" || resp.Approval.Decider != "" {
		t.Fatalf("expected empty approval when lookup fails, got %#v", resp.Approval)
	}
}
