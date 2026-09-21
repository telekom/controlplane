// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"strings"
	"testing"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// baseTarget returns a canonical Listener target used across most test cases.
func baseTarget() ctypes.TypedObjectRef {
	return ctypes.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "acp.ei.telekom.de/v1",
			Kind:       "Listener",
		},
		ObjectRef: ctypes.ObjectRef{
			Namespace: "team-ns",
			Name:      "my-listener",
			UID:       types.UID("uid-1234"),
		},
	}
}

func TestScopedApprovalName(t *testing.T) {
	// ---- Determinism / golden vectors ----
	t.Run("golden_provider", func(t *testing.T) {
		got, err := ScopedApprovalName(baseTarget(), "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-13de4a8ee48c288618c56e4f06218a9f082aa1c8a0561e65"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("golden_consumer", func(t *testing.T) {
		got, err := ScopedApprovalName(baseTarget(), "consumer")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-d60c206d4360581623a8ebd558caf0e83e511ceb596db383"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// ---- Format ----
	t.Run("format_prefix_length_dns", func(t *testing.T) {
		got, err := ScopedApprovalName(baseTarget(), "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(got, "ag-v1-") {
			t.Fatalf("expected prefix ag-v1-, got %q", got)
		}
		if len(got) != 54 {
			t.Fatalf("expected length 54, got %d (%q)", len(got), got)
		}
		// Valid DNS label: lowercase hex after prefix, no "--" in digest portion.
		digest := got[6:]
		if strings.Contains(digest, "--") {
			t.Fatalf("digest contains '--': %q", digest)
		}
		for _, c := range digest {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("non-hex char %q in digest %q", string(c), digest)
			}
		}
	})

	// ---- Domain separation ----
	t.Run("domain_separation_approval_vs_request", func(t *testing.T) {
		ag, err := ScopedApprovalName(baseTarget(), "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ar, err := ScopedApprovalRequestName(baseTarget(), "provider", "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ag[:6] == ar[:6] {
			t.Fatalf("approval and request share prefix: %q vs %q", ag[:6], ar[:6])
		}
		if ag == ar {
			t.Fatalf("approval and request names must differ")
		}
	})

	// ---- Gate separation ----
	t.Run("gate_separation_provider_vs_consumer", func(t *testing.T) {
		p, err := ScopedApprovalName(baseTarget(), "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		c, err := ScopedApprovalName(baseTarget(), "consumer")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == c {
			t.Fatalf("provider and consumer must produce different names")
		}
	})

	// ---- Target fields ----
	t.Run("target_field_group_changes_name", func(t *testing.T) {
		tgt := baseTarget()
		tgt.APIVersion = "other.group.io/v1"
		got, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-cc261254fac04f56627a000b7a3a484b52aa345036ef0dfe"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		base, _ := ScopedApprovalName(baseTarget(), "provider")
		if got == base {
			t.Fatalf("different group must produce different name")
		}
	})

	t.Run("target_field_kind_changes_name", func(t *testing.T) {
		tgt := baseTarget()
		tgt.Kind = "Application"
		got, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-c5ac7e7a5eadba952e847a894440c2bd4a6962217f6f7ce0"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("target_field_namespace_changes_name", func(t *testing.T) {
		tgt := baseTarget()
		tgt.Namespace = "other-ns"
		got, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-84a16800bb397383f9036b43b5aa00dc084047e8a9fdda55"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("target_field_name_changes_name", func(t *testing.T) {
		tgt := baseTarget()
		tgt.Name = "other-listener"
		got, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-1f2a3bd65b4e0bf1b2a64481d6309c51e64d805cec16deae"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// ---- Served version ----
	t.Run("served_version_v1_v2_same_group_same_name", func(t *testing.T) {
		v1name, err := ScopedApprovalName(baseTarget(), "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tgt := baseTarget()
		tgt.APIVersion = "acp.ei.telekom.de/v2"
		v2name, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v1name != v2name {
			t.Fatalf("same group/kind under v1 and v2 must produce same name: %q != %q", v1name, v2name)
		}
	})

	// ---- Legacy counterexample ----
	t.Run("legacy_counterexample", func(t *testing.T) {
		legacy := ApprovalName("Listener", "my-listener--provider")
		scoped, err := ScopedApprovalName(baseTarget(), "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if legacy == scoped {
			t.Fatalf("legacy and scoped names must not collide")
		}
		// Legacy contains "--", scoped does not.
		if !strings.Contains(legacy, "--") {
			t.Fatalf("legacy name must contain '--'")
		}
		if strings.Contains(scoped[6:], "--") {
			t.Fatalf("scoped digest must not contain '--'")
		}
	})

	t.Run("legacy_long_name_bounded", func(t *testing.T) {
		tgt := baseTarget()
		tgt.Namespace = "team-namespace-with-many-dashes-and-extra-segments"
		tgt.Name = "my--listener--with--double--dashes--and--extra--segments--that--exceed--normal--length"
		got, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-a0ecca56f19905249fa796a93a3b8afdb3656867d0e3cee3"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		if len(got) != 54 {
			t.Fatalf("long name must still be bounded to 54 chars, got %d", len(got))
		}
	})

	// ---- Recreation ----
	t.Run("recreation_new_uid_different_name", func(t *testing.T) {
		old, err := ScopedApprovalName(baseTarget(), "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tgt := baseTarget()
		tgt.UID = types.UID("uid-9999")
		newName, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ag-v1-674a3f8d83f61fd3a5cc3bc51405df166f7e6e7b89ebea12"
		if newName != want {
			t.Fatalf("got %q, want %q", newName, want)
		}
		if old == newName {
			t.Fatalf("new UID must produce different name (old gate cannot be inherited)")
		}
	})

	// ---- Validation ----
	t.Run("validation_empty_uid", func(t *testing.T) {
		tgt := baseTarget()
		tgt.UID = ""
		_, err := ScopedApprovalName(tgt, "provider")
		if err == nil {
			t.Fatal("expected error for empty UID")
		}
	})

	t.Run("validation_empty_key", func(t *testing.T) {
		_, err := ScopedApprovalName(baseTarget(), "")
		if err == nil {
			t.Fatal("expected error for empty key")
		}
	})

	t.Run("validation_empty_namespace", func(t *testing.T) {
		tgt := baseTarget()
		tgt.Namespace = ""
		_, err := ScopedApprovalName(tgt, "provider")
		if err == nil {
			t.Fatal("expected error for empty namespace")
		}
	})

	t.Run("validation_empty_kind", func(t *testing.T) {
		tgt := baseTarget()
		tgt.Kind = ""
		_, err := ScopedApprovalName(tgt, "provider")
		if err == nil {
			t.Fatal("expected error for empty kind")
		}
	})

	t.Run("validation_empty_apiversion", func(t *testing.T) {
		tgt := baseTarget()
		tgt.APIVersion = ""
		_, err := ScopedApprovalName(tgt, "provider")
		if err == nil {
			t.Fatal("expected error for empty APIVersion")
		}
	})

	t.Run("validation_invalid_key_uppercase", func(t *testing.T) {
		_, err := ScopedApprovalName(baseTarget(), "Provider")
		if err == nil {
			t.Fatal("expected error for uppercase key")
		}
	})

	t.Run("validation_invalid_key_too_long", func(t *testing.T) {
		_, err := ScopedApprovalName(baseTarget(), strings.Repeat("a", 33))
		if err == nil {
			t.Fatal("expected error for key > 32 chars")
		}
	})

	t.Run("validation_key_with_leading_hyphen", func(t *testing.T) {
		_, err := ScopedApprovalName(baseTarget(), "-provider")
		if err == nil {
			t.Fatal("expected error for key with leading hyphen")
		}
	})

	t.Run("validation_malformed_apiversion", func(t *testing.T) {
		tgt := baseTarget()
		tgt.APIVersion = "///invalid"
		_, err := ScopedApprovalName(tgt, "provider")
		if err == nil {
			t.Fatal("expected error for malformed APIVersion")
		}
	})

	t.Run("validation_core_group_allowed", func(t *testing.T) {
		tgt := baseTarget()
		tgt.APIVersion = "v1" // core group
		got, err := ScopedApprovalName(tgt, "provider")
		if err != nil {
			t.Fatalf("core group should be valid, got error: %v", err)
		}
		if !strings.HasPrefix(got, "ag-v1-") {
			t.Fatalf("expected ag-v1- prefix, got %q", got)
		}
	})
}

func TestScopedApprovalRequestName(t *testing.T) {
	// ---- Golden vectors ----
	t.Run("golden_intent_abc123", func(t *testing.T) {
		got, err := ScopedApprovalRequestName(baseTarget(), "provider", "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ar-v1-1860be9b609655e56b7094391a0331da30382aabc9c9cf2c"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// ---- Format ----
	t.Run("format_prefix_length", func(t *testing.T) {
		got, err := ScopedApprovalRequestName(baseTarget(), "provider", "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(got, "ar-v1-") {
			t.Fatalf("expected prefix ar-v1-, got %q", got)
		}
		if len(got) != 54 {
			t.Fatalf("expected length 54, got %d", len(got))
		}
	})

	// ---- Intent changes request name, not approval name ----
	t.Run("intent_changes_request_name", func(t *testing.T) {
		r1, err := ScopedApprovalRequestName(baseTarget(), "provider", "abc123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r2, err := ScopedApprovalRequestName(baseTarget(), "provider", "def456")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r1 == r2 {
			t.Fatalf("different intents must produce different request names")
		}

		a1, _ := ScopedApprovalName(baseTarget(), "provider")
		a2, _ := ScopedApprovalName(baseTarget(), "provider")
		if a1 != a2 {
			t.Fatalf("approval name must not change with intent")
		}
	})

	t.Run("golden_intent_def456", func(t *testing.T) {
		got, err := ScopedApprovalRequestName(baseTarget(), "provider", "def456")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "ar-v1-5602e744f99c5d6f833bf74f1f65f784e8176dfb64bf0ac3"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	// ---- Validation ----
	t.Run("validation_empty_intent_hash", func(t *testing.T) {
		_, err := ScopedApprovalRequestName(baseTarget(), "provider", "")
		if err == nil {
			t.Fatal("expected error for empty intentHash")
		}
	})

	t.Run("validation_empty_uid", func(t *testing.T) {
		tgt := baseTarget()
		tgt.UID = ""
		_, err := ScopedApprovalRequestName(tgt, "provider", "abc123")
		if err == nil {
			t.Fatal("expected error for empty UID")
		}
	})
}

func TestScopedIntentHash(t *testing.T) {
	t.Run("deterministic_string", func(t *testing.T) {
		h1, err := ScopedIntentHash("hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		h2, err := ScopedIntentHash("hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h1 != h2 {
			t.Fatalf("same input must produce same hash")
		}
		want := "5aa762ae383fbb727af3c7a36d4940a5b8c40a989452d2304fc958ff3f354e7a"
		if h1 != want {
			t.Fatalf("got %q, want %q", h1, want)
		}
	})

	t.Run("string_vs_number_distinction", func(t *testing.T) {
		hs, err := ScopedIntentHash("1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		hn, err := ScopedIntentHash(1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hs == hn {
			t.Fatalf("string '1' and number 1 must produce different hashes: %q", hs)
		}
	})

	t.Run("nil_value", func(t *testing.T) {
		got, err := ScopedIntentHash(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("map_order_independent", func(t *testing.T) {
		m1 := map[string]any{"b": 2, "a": 1}
		m2 := map[string]any{"a": 1, "b": 2}
		h1, err := ScopedIntentHash(m1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		h2, err := ScopedIntentHash(m2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h1 != h2 {
			t.Fatalf("map key order must not affect hash: %q != %q", h1, h2)
		}
		want := "43258cff783fe7036d8a43033f830adfc60ec037382473548ac742b888292777"
		if h1 != want {
			t.Fatalf("got %q, want %q", h1, want)
		}
	})

	t.Run("nested_objects", func(t *testing.T) {
		nested := map[string]any{"outer": map[string]any{"z": 3, "a": 1}}
		got, err := ScopedIntentHash(nested)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "3014e5a6baba7f8448bad687b05e5b5d46b0850ad8c372f58267467c06eec51b"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("unsupported_type_error", func(t *testing.T) {
		_, err := ScopedIntentHash(make(chan int))
		if err == nil {
			t.Fatal("expected error for unsupported type")
		}
	})
}

func TestScopedIdentityMatch(t *testing.T) {
	t.Run("same_identity", func(t *testing.T) {
		if !ScopedIdentityMatch(baseTarget(), baseTarget(), "provider", "provider") {
			t.Fatal("identical targets must match")
		}
	})

	t.Run("different_served_version_same_identity", func(t *testing.T) {
		a := baseTarget()
		b := baseTarget()
		b.APIVersion = "acp.ei.telekom.de/v2"
		if !ScopedIdentityMatch(a, b, "provider", "provider") {
			t.Fatal("same group different version must match")
		}
	})

	t.Run("different_key_no_match", func(t *testing.T) {
		if ScopedIdentityMatch(baseTarget(), baseTarget(), "provider", "consumer") {
			t.Fatal("different keys must not match")
		}
	})

	t.Run("different_uid_no_match", func(t *testing.T) {
		a := baseTarget()
		b := baseTarget()
		b.UID = types.UID("uid-other")
		if ScopedIdentityMatch(a, b, "provider", "provider") {
			t.Fatal("different UIDs must not match")
		}
	})

	t.Run("different_group_no_match", func(t *testing.T) {
		a := baseTarget()
		b := baseTarget()
		b.APIVersion = "other.group.io/v1"
		if ScopedIdentityMatch(a, b, "provider", "provider") {
			t.Fatal("different API groups must not match")
		}
	})

	t.Run("different_kind_no_match", func(t *testing.T) {
		a := baseTarget()
		b := baseTarget()
		b.Kind = "Application"
		if ScopedIdentityMatch(a, b, "provider", "provider") {
			t.Fatal("different kinds must not match")
		}
	})

	t.Run("different_namespace_no_match", func(t *testing.T) {
		a := baseTarget()
		b := baseTarget()
		b.Namespace = "other-ns"
		if ScopedIdentityMatch(a, b, "provider", "provider") {
			t.Fatal("different namespaces must not match")
		}
	})

	t.Run("different_name_no_match", func(t *testing.T) {
		a := baseTarget()
		b := baseTarget()
		b.Name = "other"
		if ScopedIdentityMatch(a, b, "provider", "provider") {
			t.Fatal("different names must not match")
		}
	})
}
