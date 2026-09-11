// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/discovery-server/internal/api"
	sstore "github.com/telekom/controlplane/discovery-server/pkg/store"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

func TestMapResponse_ResolvedApplication(t *testing.T) {
	t.Parallel()

	in := &eventv1.EventExposure{}
	in.Name = "my-app--de-telekom-eni-quickstart-v1"
	in.Namespace = "poc--eni--hyperion"
	in.Spec.EventType = "de.telekom.eni.quickstart.v1"
	in.Spec.Visibility = eventv1.VisibilityWorld
	in.Spec.Approval = eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto, TrustedTeams: []string{"eni--hyperion"}}
	in.Spec.Zone = ctypes.ObjectRef{Name: "dataplane1"}
	in.Spec.Provider = ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}}
	in.Spec.Scopes = []eventv1.EventScope{{Name: "high-priority", Trigger: eventv1.EventTrigger{ResponseFilter: &eventv1.ResponseFilter{Paths: []string{"$.data"}, Mode: eventv1.ResponseFilterModeInclude}}}}
	in.Status.Active = true
	in.Status.CallbackURL = "https://callback.example/events"
	in.Status.SseURLs = map[string]string{"dataplane1": "https://sse.example/events"}

	app := &applicationv1.Application{}
	app.Name = "my-app"
	app.Namespace = "poc--eni--hyperion"
	app.Spec.TeamEmail = "hyperion@telekom.de"

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return(app, nil)

	resp := MapResponse(context.Background(), in, &sstore.Stores{ApplicationStore: appStore})

	if resp.Visibility != api.World {
		t.Fatalf("unexpected visibility: %q", resp.Visibility)
	}
	if resp.Approval.Strategy != api.Auto {
		t.Fatalf("unexpected approval strategy: %q", resp.Approval.Strategy)
	}
	if resp.Team.Hub != "eni" || resp.Team.Name != "hyperion" {
		t.Fatalf("unexpected team mapping: %#v", resp.Team)
	}
	if resp.Application.Name != "my-app" {
		t.Fatalf("unexpected application mapping: %#v", resp.Application)
	}
	if len(resp.Scopes) != 1 || resp.Scopes[0].Name != "high-priority" {
		t.Fatalf("unexpected scopes mapping: %#v", resp.Scopes)
	}
}

func TestMapResponse_FallbackApplication(t *testing.T) {
	t.Parallel()

	in := &eventv1.EventExposure{}
	in.Name = "my-app--de-telekom-eni-quickstart-v1"
	in.Namespace = "poc--eni--hyperion"
	in.Spec.Provider = ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}}

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return((*applicationv1.Application)(nil), context.Canceled)

	resp := MapResponse(context.Background(), in, &sstore.Stores{ApplicationStore: appStore})
	if resp.Team.Hub != "eni" || resp.Team.Name != "hyperion" {
		t.Fatalf("unexpected fallback team mapping: %#v", resp.Team)
	}
}

func TestMapEventTrigger_ExpressionHandling(t *testing.T) {
	t.Parallel()

	valid := eventv1.EventTrigger{
		SelectionFilter: &eventv1.SelectionFilter{
			Expression: &apiextensionsv1.JSON{Raw: []byte(`{"op":"eq","left":"type","right":"x"}`)},
		},
	}
	mappedValid := mapEventTrigger(valid)
	if mappedValid.SelectionFilter.Expression == nil {
		t.Fatal("expected mapped expression for valid json")
	}

	invalid := eventv1.EventTrigger{
		SelectionFilter: &eventv1.SelectionFilter{
			Expression: &apiextensionsv1.JSON{Raw: []byte(`not-json`)},
		},
	}
	mappedInvalid := mapEventTrigger(invalid)
	if mappedInvalid.SelectionFilter.Expression != nil {
		t.Fatalf("expected nil expression for invalid json, got %#v", mappedInvalid.SelectionFilter.Expression)
	}
}

func TestVisibilityAndApprovalFallback(t *testing.T) {
	t.Parallel()

	if got := toAPIVisibility(eventv1.VisibilityZone); got != api.Zone {
		t.Fatalf("unexpected zone visibility mapping: %q", got)
	}
	if got := toAPIVisibility(eventv1.VisibilityEnterprise); got != api.Enterprise {
		t.Fatalf("unexpected enterprise visibility mapping: %q", got)
	}

	if got := toAPIVisibility(eventv1.Visibility("custom")); got != api.EventVisibility("CUSTOM") {
		t.Fatalf("unexpected visibility fallback: %q", got)
	}
	if got := toAPIApprovalStrategy(eventv1.ApprovalStrategySimple); got != api.Simple {
		t.Fatalf("unexpected simple approval mapping: %q", got)
	}
	if got := toAPIApprovalStrategy(eventv1.ApprovalStrategyFourEyes); got != api.FourEyes {
		t.Fatalf("unexpected four-eyes approval mapping: %q", got)
	}
	if got := toAPIApprovalStrategy(eventv1.ApprovalStrategy("custom")); got != api.EventApprovalStrategy("CUSTOM") {
		t.Fatalf("unexpected approval fallback: %q", got)
	}
}
