// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"testing"

	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

func TestApplySecurity(t *testing.T) {
	tests := []struct {
		name        string
		options     Options
		wantIssuers []string
		wantRealm   string
	}{
		{
			name:        "no options leaves security untouched",
			options:     Options{},
			wantIssuers: nil,
			wantRealm:   "",
		},
		{
			name:        "trusted issuers are sorted and de-duplicated",
			options:     Options{TrustedIssuers: []string{"b", "a", "b", "a"}},
			wantIssuers: []string{"a", "b"},
			wantRealm:   "",
		},
		{
			name:        "realm name is set when non-empty",
			options:     Options{RealmName: "my-realm"},
			wantIssuers: nil,
			wantRealm:   "my-realm",
		},
		{
			name:        "both issuers and realm are applied",
			options:     Options{TrustedIssuers: []string{"issuer-2", "issuer-1"}, RealmName: "realm-x"},
			wantIssuers: []string{"issuer-1", "issuer-2"},
			wantRealm:   "realm-x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			route := &gatewayapi.Route{}
			tc.options.applySecurity(route)

			gotIssuers := route.Spec.Security.TrustedIssuers
			if len(gotIssuers) != len(tc.wantIssuers) {
				t.Fatalf("TrustedIssuers = %v, want %v", gotIssuers, tc.wantIssuers)
			}
			for i, iss := range gotIssuers {
				if iss != tc.wantIssuers[i] {
					t.Errorf("TrustedIssuers[%d] = %q, want %q", i, iss, tc.wantIssuers[i])
				}
			}
			if route.Spec.Security.RealmName != tc.wantRealm {
				t.Errorf("RealmName = %q, want %q", route.Spec.Security.RealmName, tc.wantRealm)
			}
		})
	}
}
