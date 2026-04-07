// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/spy-server/internal/api"
	sstore "github.com/telekom/controlplane/spy-server/pkg/store"
)

func TestJoinURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		base string
		path string
		want string
	}{
		{base: "https://gw.example", path: "/a", want: "https://gw.example/a"},
		{base: "https://gw.example/", path: "a", want: "https://gw.example/a"},
		{base: "https://gw.example/", path: "/a", want: "https://gw.example/a"},
	}

	for _, tt := range tests {
		t.Run(tt.base+tt.path, func(t *testing.T) {
			if got := joinURL(tt.base, tt.path); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestMapResponse_ResolvedReferences(t *testing.T) {
	t.Parallel()

	sub := &apiv1.ApiSubscription{}
	sub.Name = "my-app--eni-distr-v1"
	sub.Namespace = "poc--eni--hyperion"
	sub.Spec.ApiBasePath = "/eni/distr/v1"
	sub.Spec.Zone = ctypes.ObjectRef{Name: "dataplane1", Namespace: "poc"}
	sub.Spec.Requestor.Application = ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}
	sub.Status.Approval = &ctypes.ObjectRef{Name: "appr", Namespace: "poc--eni--hyperion"}
	sub.Spec.Traffic.Failover = &apiv1.Failover{Zones: []ctypes.ObjectRef{{Name: "dataplane2", Namespace: "poc"}}}

	zonePrimary := &adminv1.Zone{}
	zonePrimary.Name = "dataplane1"
	zonePrimary.Namespace = "poc"
	zonePrimary.Status.Links.Url = "https://gw-primary.example/"

	zoneFailover := &adminv1.Zone{}
	zoneFailover.Name = "dataplane2"
	zoneFailover.Namespace = "poc"
	zoneFailover.Status.Links.Url = "https://gw-failover.example/"

	app := &applicationv1.Application{}
	app.Name = "my-app"
	app.Namespace = "poc--eni--hyperion"
	app.Spec.TeamEmail = "hyperion@telekom.de"

	approval := &approvalv1.Approval{}
	approval.Spec.State = approvalv1.ApprovalStateGranted
	approval.Spec.Decisions = []approvalv1.Decision{{Email: "admin@telekom.de", Comment: "approved"}}

	zoneStore := csmocks.NewMockObjectStore[*adminv1.Zone](t)
	zoneStore.EXPECT().Get(mock.Anything, "poc", "dataplane1").Return(zonePrimary, nil)
	zoneStore.EXPECT().Get(mock.Anything, "poc", "dataplane2").Return(zoneFailover, nil)

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return(app, nil)

	approvalStore := csmocks.NewMockObjectStore[*approvalv1.Approval](t)
	approvalStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "appr").Return(approval, nil)

	stores := &sstore.Stores{ZoneStore: zoneStore, ApplicationStore: appStore, ApprovalStore: approvalStore}
	resp := MapResponse(context.Background(), sub, stores)

	if resp.GatewayUrl != "https://gw-primary.example/eni/distr/v1" {
		t.Fatalf("unexpected gateway url: %q", resp.GatewayUrl)
	}
	if len(resp.Failover) != 1 || resp.Failover[0].GatewayUrl != "https://gw-failover.example/eni/distr/v1" {
		t.Fatalf("unexpected failover mapping: %#v", resp.Failover)
	}
	if resp.Team.Hub != "eni" || resp.Team.Name != "hyperion" {
		t.Fatalf("unexpected team mapping: %#v", resp.Team)
	}
	if resp.Application.Name != "my-app" {
		t.Fatalf("unexpected application mapping: %#v", resp.Application)
	}
	if resp.Approval.Decider != "admin@telekom.de" || resp.Approval.Comment != "approved" {
		t.Fatalf("unexpected approval mapping: %#v", resp.Approval)
	}
}

func TestMapResponse_MissingReferencesFallback(t *testing.T) {
	t.Parallel()

	sub := &apiv1.ApiSubscription{}
	sub.Name = "my-app--eni-distr-v1"
	sub.Namespace = "poc--eni--hyperion"
	sub.Spec.ApiBasePath = "/eni/distr/v1"
	sub.Spec.Zone = ctypes.ObjectRef{Name: "dataplane1", Namespace: "poc"}
	sub.Spec.Requestor.Application = ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}
	sub.Status.Approval = &ctypes.ObjectRef{Name: "appr", Namespace: "poc--eni--hyperion"}

	zoneStore := csmocks.NewMockObjectStore[*adminv1.Zone](t)
	zoneStore.EXPECT().Get(mock.Anything, "poc", "dataplane1").Return((*adminv1.Zone)(nil), context.Canceled)

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return((*applicationv1.Application)(nil), context.Canceled)

	approvalStore := csmocks.NewMockObjectStore[*approvalv1.Approval](t)
	approvalStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "appr").Return((*approvalv1.Approval)(nil), context.Canceled)

	stores := &sstore.Stores{ZoneStore: zoneStore, ApplicationStore: appStore, ApprovalStore: approvalStore}
	resp := MapResponse(context.Background(), sub, stores)

	if resp.GatewayUrl != "" {
		t.Fatalf("expected empty gateway url on missing zone, got %q", resp.GatewayUrl)
	}
	if resp.Team.Hub != "eni" || resp.Team.Name != "hyperion" {
		t.Fatalf("unexpected fallback team mapping: %#v", resp.Team)
	}
	if resp.Application.Name != "my-app" {
		t.Fatalf("unexpected fallback application mapping: %#v", resp.Application)
	}
	if resp.Approval.Status != "" {
		t.Fatalf("expected empty approval on missing approval ref, got %#v", resp.Approval)
	}
}

func TestMapResponse_NoApprovalReference(t *testing.T) {
	t.Parallel()

	sub := &apiv1.ApiSubscription{}
	sub.Name = "my-app--eni-distr-v1"
	sub.Namespace = "poc--eni--hyperion"
	sub.Spec.ApiBasePath = "/eni/distr/v1"
	sub.Spec.Zone = ctypes.ObjectRef{Name: "dataplane1", Namespace: "poc"}
	sub.Spec.Requestor.Application = ctypes.ObjectRef{Name: "my-app", Namespace: "poc--eni--hyperion"}
	// Intentionally keep sub.Status.Approval nil.

	zone := &adminv1.Zone{}
	zone.Name = "dataplane1"
	zone.Namespace = "poc"
	zone.Status.Links.Url = "https://gw-primary.example/"

	app := &applicationv1.Application{}
	app.Name = "my-app"
	app.Namespace = "poc--eni--hyperion"
	app.Spec.TeamEmail = "hyperion@telekom.de"

	zoneStore := csmocks.NewMockObjectStore[*adminv1.Zone](t)
	zoneStore.EXPECT().Get(mock.Anything, "poc", "dataplane1").Return(zone, nil)

	appStore := csmocks.NewMockObjectStore[*applicationv1.Application](t)
	appStore.EXPECT().Get(mock.Anything, "poc--eni--hyperion", "my-app").Return(app, nil)

	approvalStore := csmocks.NewMockObjectStore[*approvalv1.Approval](t)

	stores := &sstore.Stores{ZoneStore: zoneStore, ApplicationStore: appStore, ApprovalStore: approvalStore}
	resp := MapResponse(context.Background(), sub, stores)

	if resp.Approval.Status != "" || resp.Approval.Decider != "" || resp.Approval.Comment != "" {
		t.Fatalf("expected empty approval when approval ref is missing, got %#v", resp.Approval)
	}
}

func TestMapSecurity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(in *apiv1.ApiSubscription)
		assert func(t *testing.T, out api.ApiSubscriptionResponse)
	}{
		{
			name: "no security",
			setup: func(in *apiv1.ApiSubscription) {
				in.Spec.Security = nil
			},
			assert: func(t *testing.T, out api.ApiSubscriptionResponse) {
				t.Helper()
				// Neither variant should be set when spec has no security.
				_, errOAuth := out.Security.AsOAuth2()
				_, errBasic := out.Security.AsBasicAuth()
				if errOAuth == nil || errBasic == nil {
					t.Fatalf("expected no security to be set, but got a valid variant")
				}
			},
		},
		{
			name: "basic auth",
			setup: func(in *apiv1.ApiSubscription) {
				in.Spec.Security = &apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{Basic: &apiv1.BasicAuthCredentials{Username: "u", Password: "p"}}}
			},
			assert: func(t *testing.T, out api.ApiSubscriptionResponse) {
				t.Helper()
				got, err := out.Security.AsBasicAuth()
				if err != nil {
					t.Fatalf("expected basic auth, got err: %v", err)
				}
				if got.Username != "u" || got.Password != "p" {
					t.Fatalf("expected username=u password=p, got %#v", got)
				}
			},
		},
		{
			name: "oauth2 client",
			setup: func(in *apiv1.ApiSubscription) {
				in.Spec.Security = &apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{Client: &apiv1.OAuth2ClientCredentials{ClientId: "cid", ClientSecret: "sec"}, Scopes: []string{"s1"}}}
			},
			assert: func(t *testing.T, out api.ApiSubscriptionResponse) {
				t.Helper()
				got, err := out.Security.AsOAuth2()
				if err != nil {
					t.Fatalf("expected oauth2, got err: %v", err)
				}
				if got.ClientId != "cid" || got.ClientSecret != "sec" || len(got.Scopes) != 1 {
					t.Fatalf("unexpected oauth2 mapping: %#v", got)
				}
			},
		},
		{
			name: "oauth2 scopes only",
			setup: func(in *apiv1.ApiSubscription) {
				in.Spec.Security = &apiv1.SubscriberSecurity{M2M: &apiv1.SubscriberMachine2MachineAuthentication{Scopes: []string{"a", "b"}}}
			},
			assert: func(t *testing.T, out api.ApiSubscriptionResponse) {
				t.Helper()
				got, err := out.Security.AsOAuth2()
				if err != nil {
					t.Fatalf("expected oauth2, got err: %v", err)
				}
				if len(got.Scopes) != 2 {
					t.Fatalf("unexpected scopes mapping: %#v", got.Scopes)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := &apiv1.ApiSubscription{}
			out := api.ApiSubscriptionResponse{}
			tt.setup(in)
			mapSecurity(in, &out)
			tt.assert(t, out)
		})
	}
}
