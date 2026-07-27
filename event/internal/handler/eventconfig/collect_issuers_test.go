// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
)

func zoneWithIssuers(name, issuer, lmsIssuer string) *adminv1.Zone {
	z := &adminv1.Zone{ObjectMeta: metav1.ObjectMeta{Name: name}}
	z.Status.Links.Issuer = issuer
	z.Status.Links.LmsIssuer = lmsIssuer
	return z
}

func TestCollectPrimaryTrustedIssuers(t *testing.T) {
	tests := []struct {
		name          string
		myZone        *adminv1.Zone
		otherZones    []*adminv1.Zone
		isProxyTarget bool
		want          []string
	}{
		{
			name:          "own issuer only when not a proxy target",
			myZone:        zoneWithIssuers("zone-a", "idp-a", "lms-a"),
			otherZones:    []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b")},
			isProxyTarget: false,
			want:          []string{"idp-a"},
		},
		{
			name:          "empty own issuer is skipped",
			myZone:        zoneWithIssuers("zone-a", "", "lms-a"),
			otherZones:    nil,
			isProxyTarget: false,
			want:          nil,
		},
		{
			name:          "proxy target adds peer LMS issuers",
			myZone:        zoneWithIssuers("zone-a", "idp-a", "lms-a"),
			otherZones:    []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b"), zoneWithIssuers("zone-c", "idp-c", "lms-c")},
			isProxyTarget: true,
			want:          []string{"idp-a", "lms-b", "lms-c"},
		},
		{
			name:          "proxy target skips peers with empty LMS issuer",
			myZone:        zoneWithIssuers("zone-a", "idp-a", "lms-a"),
			otherZones:    []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", ""), zoneWithIssuers("zone-c", "idp-c", "lms-c")},
			isProxyTarget: true,
			want:          []string{"idp-a", "lms-c"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := collectPrimaryTrustedIssuers(tc.myZone, tc.otherZones, tc.isProxyTarget)
			if len(got) != len(tc.want) {
				t.Fatalf("collectPrimaryTrustedIssuers() = %v, want %v", got, tc.want)
			}
			for i, iss := range got {
				if iss != tc.want[i] {
					t.Errorf("issuer[%d] = %q, want %q", i, iss, tc.want[i])
				}
			}
		})
	}
}
