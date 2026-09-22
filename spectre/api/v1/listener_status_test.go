// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"k8s.io/apimachinery/pkg/types"
)

// sampleRef returns a populated ObjectRef for testing.
func sampleRef(name string) *ctypes.ObjectRef {
	return &ctypes.ObjectRef{
		Name:      name,
		Namespace: "ns",
		UID:       types.UID("uid-" + name),
	}
}

func TestAppliedListenerPlacementStatusRoundTrip(t *testing.T) {
	orig := &AppliedListenerPlacementStatus{
		Fingerprint:        "abc123",
		CaptureRoute:       sampleRef("route"),
		CaptureZone:        sampleRef("cap-zone"),
		CaptureEventStore:  sampleRef("cap-es"),
		CallbackOriginZone: sampleRef("cb-zone"),
		DeliveryZone:       sampleRef("del-zone"),
		DeliveryEventStore: sampleRef("del-es"),
		Publisher:          sampleRef("pub"),
		CallbackBaseURL:    "https://cb.example.com/events",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got AppliedListenerPlacementStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*orig, got) {
		t.Errorf("round-trip mismatch:\n  orig: %+v\n  got:  %+v", *orig, got)
	}
}

func TestListenerDrainStatusRoundTrip(t *testing.T) {
	orig := &ListenerDrainStatus{
		Phase:            "DrainingSubscribers",
		Reason:           "placement changed",
		OldFingerprint:   "old-fp",
		OldRouteListener: sampleRef("old-rl"),
		OldSubscribers:   []ctypes.ObjectRef{*sampleRef("sub1"), *sampleRef("sub2")},
		SourcePublisher:  sampleRef("src-pub"),
		SourceEventStore: sampleRef("src-es"),
		SubscriptionIDs:  []string{"id-1", "id-2"},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ListenerDrainStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*orig, got) {
		t.Errorf("round-trip mismatch:\n  orig: %+v\n  got:  %+v", *orig, got)
	}
}

func TestAuthorizationMigrationStatusRoundTrip(t *testing.T) {
	orig := &AuthorizationMigrationStatus{
		TargetPolicyVersion: "v2",
		Phase:               "RetiringRequests",
		LegacyApproval:      sampleRef("legacy-appr"),
		LegacyRequests:      []ctypes.ObjectRef{*sampleRef("req1")},
		RetirementCheckpoint: &MigrationRetirementCheckpoint{
			RequestsRetired:            true,
			ApprovalRetired:            false,
			LastRetiredUID:             "uid-req1",
			LastRetiredResourceVersion: "42",
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got AuthorizationMigrationStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*orig, got) {
		t.Errorf("round-trip mismatch:\n  orig: %+v\n  got:  %+v", *orig, got)
	}
}

func TestListenerStatusNewFieldsRoundTrip(t *testing.T) {
	orig := ListenerStatus{
		ConsumerApproval:           sampleRef("consumer-appr"),
		ProviderApprovalRequest:    sampleRef("prov-req"),
		ConsumerApprovalRequest:    sampleRef("cons-req"),
		AuthorizationPolicyVersion: "v2",
		AuthorizationMigration: &AuthorizationMigrationStatus{
			Phase: "Recorded",
		},
		AppliedPlacement: &AppliedListenerPlacementStatus{
			Fingerprint: "fp",
		},
		Draining: &ListenerDrainStatus{
			Phase: "Stopping",
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ListenerStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip mismatch:\n  orig: %+v\n  got:  %+v", orig, got)
	}
}

func TestOptionalFieldsNil(t *testing.T) {
	// All new fields default to nil/zero when absent.
	data := []byte(`{}`)
	var status ListenerStatus
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if status.ConsumerApproval != nil {
		t.Error("ConsumerApproval should be nil")
	}
	if status.ProviderApprovalRequest != nil {
		t.Error("ProviderApprovalRequest should be nil")
	}
	if status.ConsumerApprovalRequest != nil {
		t.Error("ConsumerApprovalRequest should be nil")
	}
	if status.AuthorizationPolicyVersion != "" {
		t.Error("AuthorizationPolicyVersion should be empty")
	}
	if status.AuthorizationMigration != nil {
		t.Error("AuthorizationMigration should be nil")
	}
	if status.AppliedPlacement != nil {
		t.Error("AppliedPlacement should be nil")
	}
	if status.Draining != nil {
		t.Error("Draining should be nil")
	}
}

func TestOptionalFieldsOmittedFromJSON(t *testing.T) {
	// When all new fields are zero, they must not appear in JSON output.
	status := ListenerStatus{}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	absent := []string{
		"consumerApproval",
		"providerApprovalRequest",
		"consumerApprovalRequest",
		"authorizationPolicyVersion",
		"authorizationMigration",
		"appliedPlacement",
		"draining",
	}
	for _, key := range absent {
		if _, ok := raw[key]; ok {
			t.Errorf("zero-value field %q should be omitted from JSON", key)
		}
	}
}

func TestPhaseEnumValuesSerialization(t *testing.T) {
	drainPhases := []string{"Stopping", "DrainingSubscribers", "CleaningPublisher", "Complete"}
	for _, phase := range drainPhases {
		d := ListenerDrainStatus{Phase: phase}
		data, err := json.Marshal(d)
		if err != nil {
			t.Errorf("marshal drain phase %q: %v", phase, err)
			continue
		}
		var got ListenerDrainStatus
		if err := json.Unmarshal(data, &got); err != nil {
			t.Errorf("unmarshal drain phase %q: %v", phase, err)
			continue
		}
		if got.Phase != phase {
			t.Errorf("drain phase round-trip: got %q, want %q", got.Phase, phase)
		}
	}

	migrationPhases := []string{"Recorded", "Blocked", "AwaitingScoped", "Draining", "RetiringRequests", "RetiringApproval"}
	for _, phase := range migrationPhases {
		m := AuthorizationMigrationStatus{Phase: phase}
		data, err := json.Marshal(m)
		if err != nil {
			t.Errorf("marshal migration phase %q: %v", phase, err)
			continue
		}
		var got AuthorizationMigrationStatus
		if err := json.Unmarshal(data, &got); err != nil {
			t.Errorf("unmarshal migration phase %q: %v", phase, err)
			continue
		}
		if got.Phase != phase {
			t.Errorf("migration phase round-trip: got %q, want %q", got.Phase, phase)
		}
	}
}

func TestNoEmbeddedFullObjects(t *testing.T) {
	// Structural assertion: status types only reference ObjectRef, not full
	// K8s objects. Walk all fields and verify none embed metav1.ObjectMeta
	// (a proxy for "this is a full K8s resource").
	statusTypes := []reflect.Type{
		reflect.TypeOf(AppliedListenerPlacementStatus{}),
		reflect.TypeOf(ListenerDrainStatus{}),
		reflect.TypeOf(AuthorizationMigrationStatus{}),
		reflect.TypeOf(MigrationRetirementCheckpoint{}),
	}
	for _, st := range statusTypes {
		assertNoEmbeddedObjectMeta(t, st, st.Name())
	}
}

func assertNoEmbeddedObjectMeta(t *testing.T, rt reflect.Type, path string) {
	t.Helper()
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		fieldPath := path + "." + f.Name
		if ft.Name() == "ObjectMeta" {
			t.Errorf("field %s embeds ObjectMeta (full K8s object), want ObjectRef", fieldPath)
		}
		if ft.Kind() == reflect.Struct && ft.Name() != "ObjectRef" && ft.Name() != "TypedObjectRef" {
			assertNoEmbeddedObjectMeta(t, ft, fieldPath)
		}
	}
}

func TestDeepCopyNewStatusTypes(t *testing.T) {
	orig := &ListenerStatus{
		ConsumerApproval:           sampleRef("ca"),
		ProviderApprovalRequest:    sampleRef("par"),
		ConsumerApprovalRequest:    sampleRef("car"),
		AuthorizationPolicyVersion: "v2",
		AuthorizationMigration: &AuthorizationMigrationStatus{
			TargetPolicyVersion: "v2",
			Phase:               "Draining",
			LegacyApproval:      sampleRef("la"),
			LegacyRequests:      []ctypes.ObjectRef{*sampleRef("lr1")},
			RetirementCheckpoint: &MigrationRetirementCheckpoint{
				RequestsRetired: true,
				LastRetiredUID:  "uid-1",
			},
		},
		AppliedPlacement: &AppliedListenerPlacementStatus{
			Fingerprint:  "fp1",
			CaptureRoute: sampleRef("cr"),
			Publisher:    sampleRef("pub"),
		},
		Draining: &ListenerDrainStatus{
			Phase:           "DrainingSubscribers",
			OldSubscribers:  []ctypes.ObjectRef{*sampleRef("os1")},
			SubscriptionIDs: []string{"sid-1"},
		},
	}

	cp := orig.DeepCopy()

	// Verify structural equality.
	if !reflect.DeepEqual(orig, cp) {
		t.Fatal("DeepCopy produced a non-equal copy")
	}

	// Verify independence: mutating the copy must not affect the original.
	cp.ConsumerApproval.Name = "mutated"
	if orig.ConsumerApproval.Name == "mutated" {
		t.Error("ConsumerApproval not deep-copied")
	}
	cp.AuthorizationMigration.Phase = "Blocked"
	if orig.AuthorizationMigration.Phase == "Blocked" {
		t.Error("AuthorizationMigration not deep-copied")
	}
	cp.AppliedPlacement.Fingerprint = "mutated"
	if orig.AppliedPlacement.Fingerprint == "mutated" {
		t.Error("AppliedPlacement not deep-copied")
	}
	cp.Draining.SubscriptionIDs[0] = "mutated"
	if orig.Draining.SubscriptionIDs[0] == "mutated" {
		t.Error("Draining.SubscriptionIDs not deep-copied")
	}
	cp.AuthorizationMigration.LegacyRequests[0].Name = "mutated"
	if orig.AuthorizationMigration.LegacyRequests[0].Name == "mutated" {
		t.Error("AuthorizationMigration.LegacyRequests not deep-copied")
	}
}
