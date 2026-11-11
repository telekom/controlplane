// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package pagination

import "testing"

func TestPaginateLinks(t *testing.T) {
	t.Parallel()

	items := []int{1, 2, 3, 4, 5}

	tests := []struct {
		name       string
		basePath   string
		expectSelf string
		expectNext string
	}{
		{
			name:       "plain base path",
			basePath:   "/applications",
			expectSelf: "/applications?offset=2&limit=2",
			expectNext: "/applications?offset=4&limit=2",
		},
		{
			name:       "base path with query keeps it",
			basePath:   "/applications?icto=icto-12345",
			expectSelf: "/applications?icto=icto-12345&offset=2&limit=2",
			expectNext: "/applications?icto=icto-12345&offset=4&limit=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := Paginate(items, 2, 2, tt.basePath)
			if res.Links.Self != tt.expectSelf {
				t.Fatalf("expected self %q, got %q", tt.expectSelf, res.Links.Self)
			}
			if res.Links.Next != tt.expectNext {
				t.Fatalf("expected next %q, got %q", tt.expectNext, res.Links.Next)
			}
		})
	}
}
