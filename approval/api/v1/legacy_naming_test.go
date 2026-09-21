// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"testing"

	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/util/hash"
)

// TestLegacyApprovalNames freezes the legacy naming convention for Approval and
// ApprovalRequest resources. Phase 2 migration tests compare against these
// baselines to guarantee the old names are preserved during the dual-gate
// transition.
func TestLegacyApprovalNames(t *testing.T) {
	t.Run("ApprovalName/Listener", func(t *testing.T) {
		got := ApprovalName("Listener", "my-listener")
		want := "listener--my-listener"
		if got != want {
			t.Fatalf("ApprovalName(Listener, my-listener) = %q, want %q", got, want)
		}
	})

	t.Run("ApprovalName/TestResource", func(t *testing.T) {
		got := ApprovalName("TestResource", "my-test-resource")
		want := "testresource--my-test-resource"
		if got != want {
			t.Fatalf("ApprovalName(TestResource, my-test-resource) = %q, want %q", got, want)
		}
	})

	t.Run("ApprovalName/ApiSubscription", func(t *testing.T) {
		got := ApprovalName("ApiSubscription", "my-api-sub")
		want := "apisubscription--my-api-sub"
		if got != want {
			t.Fatalf("ApprovalName(ApiSubscription, my-api-sub) = %q, want %q", got, want)
		}
	})

	t.Run("ApprovalName/owner_with_double_dash_provider", func(t *testing.T) {
		got := ApprovalName("Listener", "team--provider--app")
		want := "listener--team--provider--app"
		if got != want {
			t.Fatalf("ApprovalName(Listener, team--provider--app) = %q, want %q", got, want)
		}
	})

	t.Run("ApprovalName/long_name_within_accepted_range", func(t *testing.T) {
		longName := make([]byte, 200)
		for i := range longName {
			longName[i] = 'a'
		}
		got := ApprovalName("Listener", string(longName))
		want := "listener--" + string(longName)
		if got != want {
			t.Fatalf("ApprovalName with 200-char owner name mismatch: len(got)=%d, len(want)=%d", len(got), len(want))
		}
	})

	t.Run("ApprovalName/stable_repeated_calls", func(t *testing.T) {
		first := ApprovalName("Listener", "stable-test")
		second := ApprovalName("Listener", "stable-test")
		if first != second {
			t.Fatalf("ApprovalName not stable: %q != %q", first, second)
		}
	})

	t.Run("ApprovalRequestName/TestResource_empty_spec", func(t *testing.T) {
		owner := test.NewObject("my-test-resource", "default")
		spec := test.TestResourceSpec{}
		got := ApprovalRequestName(owner, spec)
		want := "my-test-resource--85758569cf"
		if got != want {
			t.Fatalf("ApprovalRequestName(my-test-resource, emptySpec) = %q, want %q", got, want)
		}
	})

	t.Run("ApprovalRequestName/representative_hash_from_callers", func(t *testing.T) {
		owner := test.NewObject("fixture-listener", "team-ns")
		hashValue := "same-team-fingerprint-value"
		got := ApprovalRequestName(owner, hashValue)
		want := "fixture-listener--55b548c9f4"
		if got != want {
			t.Fatalf("ApprovalRequestName(fixture-listener, fingerprint) = %q, want %q", got, want)
		}
	})

	t.Run("ApprovalRequestName/stable_repeated_calls", func(t *testing.T) {
		owner := test.NewObject("repeat-owner", "ns")
		first := ApprovalRequestName(owner, "deterministic")
		second := ApprovalRequestName(owner, "deterministic")
		if first != second {
			t.Fatalf("ApprovalRequestName not stable: %q != %q", first, second)
		}
	})

	t.Run("ComputeHash/baseline_vectors", func(t *testing.T) {
		spec := test.TestResourceSpec{}
		got := hash.ComputeHash(&spec, nil)
		want := "85758569cf"
		if got != want {
			t.Fatalf("ComputeHash(emptySpec) = %q, want %q", got, want)
		}
	})
}
