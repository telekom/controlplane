// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package file

import (
	"testing"

	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func TestMakeName(t *testing.T) {
	tests := []struct {
		name     string
		fileType string
		owner    string
		want     string
	}{
		{"hyphenated file type", "de-telekom-eni-foo-v1", "provider", "de-telekom-eni-foo-v1--provider"},
		{"dotted file type is normalized", "de.telekom.foo.v1", "consumer", "de-telekom-foo-v1--consumer"},
		{"mixed case is lowercased", "De.Telekom.V1", "app", "de-telekom-v1--app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MakeName(tt.fileType, tt.owner); got != tt.want {
				t.Errorf("MakeName(%q, %q) = %q, want %q", tt.fileType, tt.owner, got, tt.want)
			}
		})
	}
}

func TestMapPublicKeys(t *testing.T) {
	t.Run("nil input yields nil", func(t *testing.T) {
		if got := mapPublicKeys(nil); got != nil {
			t.Errorf("mapPublicKeys(nil) = %v, want nil", got)
		}
	})

	t.Run("maps label and key preserving order", func(t *testing.T) {
		in := []roverv1.PublicKey{
			{Label: "provider-key", Key: "ssh-ed25519 AAAA"},
			{Label: "consumer-key", Key: "ssh-ed25519 BBBB"},
		}
		got := mapPublicKeys(in)
		if len(got) != 2 {
			t.Fatalf("mapPublicKeys len = %d, want 2", len(got))
		}
		if got[0].Label != "provider-key" || got[0].Key != "ssh-ed25519 AAAA" {
			t.Errorf("got[0] = %+v", got[0])
		}
		if got[1].Label != "consumer-key" || got[1].Key != "ssh-ed25519 BBBB" {
			t.Errorf("got[1] = %+v", got[1])
		}
	})
}
