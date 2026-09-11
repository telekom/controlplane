// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventtype

import (
	"testing"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

func TestMapResponse(t *testing.T) {
	t.Parallel()

	in := &eventv1.EventType{}
	in.Name = "de-telekom-eni-quickstart-v1"
	in.Namespace = "poc--eni--hyperion"
	in.Spec.Type = "de.telekom.eni.quickstart.v1"
	in.Spec.Version = "1.0.0"
	in.Spec.Description = "Quickstart event type"
	in.Spec.Specification = "spec"
	in.Status.Active = true

	resp := MapResponse(in)

	if resp.Name != "de-telekom-eni-quickstart-v1" {
		t.Fatalf("unexpected name: %q", resp.Name)
	}
	if resp.Id != "eni--hyperion--de-telekom-eni-quickstart-v1" {
		t.Fatalf("unexpected id: %q", resp.Id)
	}
	if resp.Type != "de.telekom.eni.quickstart.v1" || resp.Version != "1.0.0" {
		t.Fatalf("unexpected type/version mapping: %#v", resp)
	}
	if !resp.Active {
		t.Fatal("expected active event type")
	}
}
