// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure

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
		name            string
		zone            *adminv1.Zone
		subscriberZones []*adminv1.Zone
		isProxyTarget   bool
		want            []string
	}{
		{
			name:            "own issuer only when not a proxy target",
			zone:            zoneWithIssuers("zone-a", "idp-a", "lms-a"),
			subscriberZones: []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b")},
			isProxyTarget:   false,
			want:            []string{"idp-a"},
		},
		{
			name:            "empty own issuer is skipped",
			zone:            zoneWithIssuers("zone-a", "", "lms-a"),
			subscriberZones: nil,
			isProxyTarget:   false,
			want:            nil,
		},
		{
			name:            "proxy target adds subscriber LMS issuers",
			zone:            zoneWithIssuers("zone-a", "idp-a", "lms-a"),
			subscriberZones: []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", "lms-b"), zoneWithIssuers("zone-c", "idp-c", "lms-c")},
			isProxyTarget:   true,
			want:            []string{"idp-a", "lms-b", "lms-c"},
		},
		{
			name:            "proxy target skips subscribers with empty LMS issuer",
			zone:            zoneWithIssuers("zone-a", "idp-a", "lms-a"),
			subscriberZones: []*adminv1.Zone{zoneWithIssuers("zone-b", "idp-b", ""), zoneWithIssuers("zone-c", "idp-c", "lms-c")},
			isProxyTarget:   true,
			want:            []string{"idp-a", "lms-c"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := collectPrimaryTrustedIssuers(tc.zone, tc.subscriberZones, tc.isProxyTarget)
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
