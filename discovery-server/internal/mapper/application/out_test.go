// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	commonTypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/discovery-server/internal/api"
)

func TestMapResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		in         *applicationv1.Application
		expectID   string
		expectName string
		expectHub  string
		expectTeam string
		expectZone string
		assert     func(t *testing.T, resp api.ApplicationResponse)
	}{
		{
			name: "maps canonical id and core fields",
			in: &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "poc--eni--hyperion",
				},
				Spec: applicationv1.ApplicationSpec{
					Team:      "hyperion",
					TeamEmail: "hyperion@telekom.de",
					Zone:      commonTypes.ObjectRef{Name: "dataplane1"},
					Security: &applicationv1.Security{
						IpRestrictions: &applicationv1.IpRestrictions{
							Allow: []string{"10.0.0.0/8", "172.16.0.0/12"},
						},
					},
				},
			},
			expectID:   "eni--hyperion--my-app",
			expectName: "my-app",
			expectHub:  "eni",
			expectTeam: "hyperion",
			expectZone: "dataplane1",
			assert: func(t *testing.T, resp api.ApplicationResponse) {
				t.Helper()
				if len(resp.Security.IpRestrictions.Allow) != 2 {
					t.Fatalf("expected 2 allowed cidrs, got %d", len(resp.Security.IpRestrictions.Allow))
				}
			},
		},
		{
			name: "without security leaves security empty",
			in: &applicationv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "poc--eni--hyperion",
				},
				Spec: applicationv1.ApplicationSpec{
					Team:      "hyperion",
					TeamEmail: "hyperion@telekom.de",
					Zone:      commonTypes.ObjectRef{Name: "dataplane1"},
					Security:  nil,
				},
			},
			expectID:   "eni--hyperion--my-app",
			expectName: "my-app",
			expectHub:  "eni",
			expectTeam: "hyperion",
			expectZone: "dataplane1",
			assert: func(t *testing.T, resp api.ApplicationResponse) {
				t.Helper()
				if len(resp.Security.IpRestrictions.Allow) != 0 {
					t.Fatalf("expected empty ip restrictions when source has no security, got %#v", resp.Security.IpRestrictions.Allow)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := MapResponse(tt.in)

			if resp.Id != tt.expectID {
				t.Fatalf("expected id %q, got %q", tt.expectID, resp.Id)
			}
			if resp.Name != tt.expectName {
				t.Fatalf("expected name %q, got %q", tt.expectName, resp.Name)
			}
			if resp.Team.Hub != tt.expectHub {
				t.Fatalf("expected hub %q, got %q", tt.expectHub, resp.Team.Hub)
			}
			if resp.Team.Name != tt.expectTeam {
				t.Fatalf("expected team %q, got %q", tt.expectTeam, resp.Team.Name)
			}
			if resp.Zone != tt.expectZone {
				t.Fatalf("expected zone %q, got %q", tt.expectZone, resp.Zone)
			}

			tt.assert(t, resp)
		})
	}
}
