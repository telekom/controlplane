// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package identifier

import "testing"

func TestConvertFileIdToPath(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "UUID stays as a bare key",
			id:   "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60",
			want: "01926a3e-7b2c-7d3e-8f4a-1b2c3d4e5f60",
		},
		{
			name: "legacy ID maps its resource prefix to folders",
			id:   "poc--eni--hyperion--my-test-file.txt",
			want: "poc/eni/hyperion/my-test-file.txt",
		},
		{
			name: "legacy filename double dashes remain unchanged",
			id:   "poc--eni--hyperion--file--revision.txt",
			want: "poc/eni/hyperion/file--revision.txt",
		},
		{
			name: "unrecognized IDs pass through",
			id:   "invalid-id",
			want: "invalid-id",
		},
		{
			name: "malformed legacy ID passes through",
			id:   "poc----hyperion--file.txt",
			want: "poc----hyperion--file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertFileIdToPath(tt.id); got != tt.want {
				t.Errorf("ConvertFileIdToPath(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
