// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"testing"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func TestCollectZones(t *testing.T) {
	zoneA := &adminv1.Zone{ObjectMeta: metav1.ObjectMeta{Name: "zone-a"}}
	zoneB := &adminv1.Zone{ObjectMeta: metav1.ObjectMeta{Name: "zone-b"}}
	zoneC := &adminv1.Zone{ObjectMeta: metav1.ObjectMeta{Name: "zone-c"}}

	candidates := []*adminv1.Zone{zoneA, zoneB, zoneC}

	tests := []struct {
		name       string
		candidates []*adminv1.Zone
		fullMesh   bool
		wanted     []string
		wantNames  []string
	}{
		{
			name:       "fullMesh=true returns all candidates",
			candidates: candidates,
			fullMesh:   true,
			wanted:     nil,
			wantNames:  []string{"zone-a", "zone-b", "zone-c"},
		},
		{
			name:       "fullMesh=false with matching wanted names filters correctly",
			candidates: candidates,
			fullMesh:   false,
			wanted:     []string{"zone-a", "zone-c"},
			wantNames:  []string{"zone-a", "zone-c"},
		},
		{
			name:       "fullMesh=false with empty wanted list returns empty",
			candidates: candidates,
			fullMesh:   false,
			wanted:     []string{},
			wantNames:  nil,
		},
		{
			name:       "fullMesh=false with no matching candidates returns empty",
			candidates: candidates,
			fullMesh:   false,
			wanted:     []string{"zone-x", "zone-y"},
			wantNames:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := collectZones(tc.candidates, tc.fullMesh, tc.wanted)

			if len(got) != len(tc.wantNames) {
				t.Fatalf("collectZones() returned %d zones, want %d", len(got), len(tc.wantNames))
			}

			for i, zone := range got {
				if zone.Name != tc.wantNames[i] {
					t.Errorf("collectZones()[%d].Name = %q, want %q", i, zone.Name, tc.wantNames[i])
				}
			}
		})
	}
}

func TestOptionsApply(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(t *testing.T) (*fakeclient.MockJanitorClient, context.Context)
		options   Options
		route     *gatewayapi.Route
		wantErr   bool
		wantOwner bool
	}{
		{
			name: "Owner is nil returns nil without calling Scheme",
			setupMock: func(t *testing.T) (*fakeclient.MockJanitorClient, context.Context) {
				mockClient := fakeclient.NewMockJanitorClient(t)
				mockClient.EXPECT().Scheme().Return(nil).Maybe()
				ctx := cclient.WithClient(context.Background(), mockClient)
				return mockClient, ctx
			},
			options: Options{Owner: nil},
			route: &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
			},
			wantErr:   false,
			wantOwner: false,
		},
		{
			name: "Owner set with proper scheme sets owner reference",
			setupMock: func(t *testing.T) (*fakeclient.MockJanitorClient, context.Context) {
				s := runtime.NewScheme()
				if err := eventv1.AddToScheme(s); err != nil {
					t.Fatalf("failed to add eventv1 to scheme: %v", err)
				}
				if err := gatewayapi.AddToScheme(s); err != nil {
					t.Fatalf("failed to add gatewayapi to scheme: %v", err)
				}
				mockClient := fakeclient.NewMockJanitorClient(t)
				mockClient.EXPECT().Scheme().Return(s).Maybe()
				ctx := cclient.WithClient(context.Background(), mockClient)
				return mockClient, ctx
			},
			options: Options{
				Owner: &eventv1.EventConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eventconfig",
						Namespace: "default",
						UID:       types.UID("test-uid-1234"),
					},
				},
			},
			route: &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
			},
			wantErr:   false,
			wantOwner: true,
		},
		{
			name: "Owner set but scheme missing owner type returns error",
			setupMock: func(t *testing.T) (*fakeclient.MockJanitorClient, context.Context) {
				emptyScheme := runtime.NewScheme()
				mockClient := fakeclient.NewMockJanitorClient(t)
				mockClient.EXPECT().Scheme().Return(emptyScheme).Maybe()
				ctx := cclient.WithClient(context.Background(), mockClient)
				return mockClient, ctx
			},
			options: Options{
				Owner: &eventv1.EventConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eventconfig",
						Namespace: "default",
						UID:       types.UID("test-uid-1234"),
					},
				},
			},
			route: &gatewayapi.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
			},
			wantErr:   true,
			wantOwner: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ctx := tc.setupMock(t)

			err := tc.options.apply(ctx, tc.route)

			if tc.wantErr && err == nil {
				t.Fatal("apply() expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("apply() unexpected error: %v", err)
			}

			ownerRefs := tc.route.GetOwnerReferences()
			if tc.wantOwner {
				if len(ownerRefs) == 0 {
					t.Fatal("apply() expected owner reference to be set, but none found")
				}
				if ownerRefs[0].Name != tc.options.Owner.GetName() {
					t.Errorf("owner reference name = %q, want %q", ownerRefs[0].Name, tc.options.Owner.GetName())
				}
				if ownerRefs[0].UID != tc.options.Owner.GetUID() {
					t.Errorf("owner reference UID = %q, want %q", ownerRefs[0].UID, tc.options.Owner.GetUID())
				}
			} else {
				if len(ownerRefs) != 0 {
					t.Errorf("apply() expected no owner references, got %d", len(ownerRefs))
				}
			}
		})
	}
}
