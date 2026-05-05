// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"encoding/json"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
)

// helper: build a JSON expression from a Go map.
func mustJSON(t *testing.T, v any) *apiextensionsv1.JSON {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

// helper: unmarshal a *apiextensionsv1.JSON into a generic map.
func jsonToMap(t *testing.T, j *apiextensionsv1.JSON) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(j.Raw, &m); err != nil {
		t.Fatalf("jsonToMap: %v", err)
	}
	return m
}

// helper: build an EventExposure with given scopes.
func makeExposure(scopes []eventv1.EventScope) *eventv1.EventExposure {
	return &eventv1.EventExposure{
		Spec: eventv1.EventExposureSpec{
			Scopes: scopes,
		},
	}
}

// =========================================================================
// attributesToExpression tests
// =========================================================================

func TestAttributesToExpression_SingleAttribute(t *testing.T) {
	result := attributesToExpression(map[string]string{"color": "red"})

	// Expect {"eq": {"field": "color", "value": "red"}}
	eq, ok := result["eq"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'eq' key, got: %v", result)
	}
	if eq["field"] != "color" || eq["value"] != "red" {
		t.Errorf("expected field=color, value=red, got field=%v, value=%v", eq["field"], eq["value"])
	}
}

func TestAttributesToExpression_MultipleAttributes(t *testing.T) {
	result := attributesToExpression(map[string]string{"color": "red", "size": "large"})

	// Expect {"and": [{"eq": {"field":"color","value":"red"}}, {"eq": {"field":"size","value":"large"}}]}
	andList, ok := result["and"].([]any)
	if !ok {
		t.Fatalf("expected 'and' key, got: %v", result)
	}
	if len(andList) != 2 {
		t.Fatalf("expected 2 items in 'and', got %d", len(andList))
	}

	// Keys are sorted, so "color" comes before "size"
	first := andList[0].(map[string]any)["eq"].(map[string]any)
	second := andList[1].(map[string]any)["eq"].(map[string]any)

	if first["field"] != "color" || first["value"] != "red" {
		t.Errorf("first eq: expected color=red, got %v=%v", first["field"], first["value"])
	}
	if second["field"] != "size" || second["value"] != "large" {
		t.Errorf("second eq: expected size=large, got %v=%v", second["field"], second["value"])
	}
}

func TestAttributesToExpression_DeterministicOrdering(t *testing.T) {
	attrs := map[string]string{"z": "1", "a": "2", "m": "3"}

	// Run multiple times to verify determinism
	for i := 0; i < 10; i++ {
		result := attributesToExpression(attrs)
		andList := result["and"].([]any)

		fields := make([]string, len(andList))
		for j, item := range andList {
			eq := item.(map[string]any)["eq"].(map[string]any)
			fields[j] = eq["field"].(string)
		}

		if fields[0] != "a" || fields[1] != "m" || fields[2] != "z" {
			t.Errorf("iteration %d: expected sorted [a,m,z], got %v", i, fields)
		}
	}
}

// =========================================================================
// finalizeSelectionFilter tests
// =========================================================================

func TestFinalizeSelectionFilter_NoExpressions(t *testing.T) {
	result := &eventv1.EventTrigger{}
	finalizeSelectionFilter(result, nil)

	if result.SelectionFilter != nil {
		t.Errorf("expected nil SelectionFilter for empty expressions")
	}
}

func TestFinalizeSelectionFilter_SingleExpression(t *testing.T) {
	result := &eventv1.EventTrigger{}
	expr := map[string]any{"eq": map[string]any{"field": "type", "value": "A"}}
	finalizeSelectionFilter(result, []map[string]any{expr})

	if result.SelectionFilter == nil || result.SelectionFilter.Expression == nil {
		t.Fatal("expected SelectionFilter.Expression to be set")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	// Should be the expression directly, not wrapped in OR
	if _, hasOr := got["or"]; hasOr {
		t.Error("single expression should not be wrapped in 'or'")
	}
	eq, ok := got["eq"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'eq' key, got: %v", got)
	}
	if eq["field"] != "type" || eq["value"] != "A" {
		t.Errorf("expected field=type, value=A, got %v", eq)
	}
}

func TestFinalizeSelectionFilter_MultipleExpressions(t *testing.T) {
	result := &eventv1.EventTrigger{}
	expr1 := map[string]any{"eq": map[string]any{"field": "type", "value": "A"}}
	expr2 := map[string]any{"eq": map[string]any{"field": "type", "value": "B"}}
	finalizeSelectionFilter(result, []map[string]any{expr1, expr2})

	if result.SelectionFilter == nil || result.SelectionFilter.Expression == nil {
		t.Fatal("expected SelectionFilter.Expression to be set")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	orList, ok := got["or"].([]any)
	if !ok {
		t.Fatalf("expected 'or' key for multiple expressions, got: %v", got)
	}
	if len(orList) != 2 {
		t.Fatalf("expected 2 items in 'or', got %d", len(orList))
	}
}

// =========================================================================
// deduplicateResponseFilterPaths tests
// =========================================================================

func TestDeduplicateResponseFilterPaths_NilResponseFilter(t *testing.T) {
	result := &eventv1.EventTrigger{}
	deduplicateResponseFilterPaths(result)
	// no-op, no panic
	if result.ResponseFilter != nil {
		t.Error("expected nil ResponseFilter")
	}
}

func TestDeduplicateResponseFilterPaths_EmptyPaths(t *testing.T) {
	result := &eventv1.EventTrigger{
		ResponseFilter: &eventv1.ResponseFilter{Paths: []string{}},
	}
	deduplicateResponseFilterPaths(result)
	if len(result.ResponseFilter.Paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(result.ResponseFilter.Paths))
	}
}

func TestDeduplicateResponseFilterPaths_NoDuplicates(t *testing.T) {
	result := &eventv1.EventTrigger{
		ResponseFilter: &eventv1.ResponseFilter{Paths: []string{"a", "b", "c"}},
	}
	deduplicateResponseFilterPaths(result)
	if len(result.ResponseFilter.Paths) != 3 {
		t.Errorf("expected 3 paths, got %d", len(result.ResponseFilter.Paths))
	}
}

func TestDeduplicateResponseFilterPaths_WithDuplicates(t *testing.T) {
	result := &eventv1.EventTrigger{
		ResponseFilter: &eventv1.ResponseFilter{Paths: []string{"a", "b", "a", "c", "b"}},
	}
	deduplicateResponseFilterPaths(result)

	expected := []string{"a", "b", "c"}
	if len(result.ResponseFilter.Paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(result.ResponseFilter.Paths), result.ResponseFilter.Paths)
	}
	for i, p := range result.ResponseFilter.Paths {
		if p != expected[i] {
			t.Errorf("path[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

func TestDeduplicateResponseFilterPaths_PreservesOrder(t *testing.T) {
	result := &eventv1.EventTrigger{
		ResponseFilter: &eventv1.ResponseFilter{Paths: []string{"c", "a", "c", "b", "a"}},
	}
	deduplicateResponseFilterPaths(result)

	// First occurrence order: c, a, b
	expected := []string{"c", "a", "b"}
	if len(result.ResponseFilter.Paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d", len(expected), len(result.ResponseFilter.Paths))
	}
	for i, p := range result.ResponseFilter.Paths {
		if p != expected[i] {
			t.Errorf("path[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

// =========================================================================
// createPublisherTrigger tests
// =========================================================================

func TestCreatePublisherTrigger_NoSubscribedScopes(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{Attributes: map[string]string{"type": "A"}},
		}},
	})
	result := createPublisherTrigger(exposure, []string{})
	if result != nil {
		t.Errorf("expected nil for empty subscribed scopes, got: %v", result)
	}
}

func TestCreatePublisherTrigger_NoExposureScopes(t *testing.T) {
	exposure := makeExposure(nil)
	result := createPublisherTrigger(exposure, []string{"gold"})
	if result != nil {
		t.Errorf("expected nil for empty exposure scopes, got: %v", result)
	}
}

func TestCreatePublisherTrigger_ScopeNotFound(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{Attributes: map[string]string{"type": "A"}},
		}},
	})
	result := createPublisherTrigger(exposure, []string{"missing"})
	if result != nil {
		t.Errorf("expected nil when no scope names match, got: %v", result)
	}
}

func TestCreatePublisherTrigger_SingleScope_AttributesOnly(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"color": "red"},
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SelectionFilter == nil || result.SelectionFilter.Expression == nil {
		t.Fatal("expected SelectionFilter.Expression to be set")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	// Single attribute → {"eq": {"field": "color", "value": "red"}}
	eq, ok := got["eq"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'eq', got: %v", got)
	}
	if eq["field"] != "color" || eq["value"] != "red" {
		t.Errorf("unexpected eq: %v", eq)
	}
	if result.ResponseFilter != nil {
		t.Error("expected nil ResponseFilter")
	}
}

func TestCreatePublisherTrigger_SingleScope_ExpressionOnly(t *testing.T) {
	exprMap := map[string]any{"gt": map[string]any{"field": "age", "value": float64(18)}}
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Expression: mustJSON(t, exprMap),
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)
	gt, ok := got["gt"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'gt', got: %v", got)
	}
	if gt["field"] != "age" {
		t.Errorf("expected field=age, got %v", gt["field"])
	}
}

func TestCreatePublisherTrigger_SingleScope_MultiAttributes(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"color": "red", "size": "big"},
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	// Multiple attributes → {"and": [{"eq":...color}, {"eq":...size}]}
	andList, ok := got["and"].([]any)
	if !ok {
		t.Fatalf("expected 'and' for multi-attrs, got: %v", got)
	}
	if len(andList) != 2 {
		t.Fatalf("expected 2 items in 'and', got %d", len(andList))
	}
}

func TestCreatePublisherTrigger_TwoScopes_BothAttributes(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "A"},
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "B"},
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold", "silver"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	// Two scopes → {"or": [scope1_expr, scope2_expr]}
	orList, ok := got["or"].([]any)
	if !ok {
		t.Fatalf("expected 'or' for two scopes, got: %v", got)
	}
	if len(orList) != 2 {
		t.Fatalf("expected 2 items in 'or', got %d", len(orList))
	}

	// First: {"eq": {"field":"type","value":"A"}}
	first := orList[0].(map[string]any)
	eq1 := first["eq"].(map[string]any)
	if eq1["value"] != "A" {
		t.Errorf("first scope: expected value=A, got %v", eq1["value"])
	}

	// Second: {"eq": {"field":"type","value":"B"}}
	second := orList[1].(map[string]any)
	eq2 := second["eq"].(map[string]any)
	if eq2["value"] != "B" {
		t.Errorf("second scope: expected value=B, got %v", eq2["value"])
	}
}

func TestCreatePublisherTrigger_TwoScopes_MixedAttrsAndExpression(t *testing.T) {
	customExpr := map[string]any{"gt": map[string]any{"field": "priority", "value": float64(5)}}
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "A"},
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Expression: mustJSON(t, customExpr),
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold", "silver"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	orList, ok := got["or"].([]any)
	if !ok {
		t.Fatalf("expected 'or', got: %v", got)
	}
	if len(orList) != 2 {
		t.Fatalf("expected 2 items in 'or', got %d", len(orList))
	}

	// First: converted attributes → {"eq":...}
	first := orList[0].(map[string]any)
	if _, ok := first["eq"]; !ok {
		t.Errorf("first scope should be eq expression from attributes, got: %v", first)
	}

	// Second: pass-through expression → {"gt":...}
	second := orList[1].(map[string]any)
	if _, ok := second["gt"]; !ok {
		t.Errorf("second scope should be gt expression, got: %v", second)
	}
}

func TestCreatePublisherTrigger_ResponseFilterPathsUnion(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.a"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.b"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold", "silver"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ResponseFilter == nil {
		t.Fatal("expected ResponseFilter to be set")
	}

	expected := []string{"$.data.a", "$.data.b"}
	if len(result.ResponseFilter.Paths) != len(expected) {
		t.Fatalf("expected %d paths, got %d: %v", len(expected), len(result.ResponseFilter.Paths), result.ResponseFilter.Paths)
	}
	for i, p := range result.ResponseFilter.Paths {
		if p != expected[i] {
			t.Errorf("path[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

func TestCreatePublisherTrigger_ResponseFilterPathsDedup(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.a"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.a"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold", "silver"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.ResponseFilter.Paths) != 1 {
		t.Errorf("expected 1 deduplicated path, got %d: %v", len(result.ResponseFilter.Paths), result.ResponseFilter.Paths)
	}
	if result.ResponseFilter.Paths[0] != "$.data.a" {
		t.Errorf("expected $.data.a, got %s", result.ResponseFilter.Paths[0])
	}
}

func TestCreatePublisherTrigger_ResponseFilterModeLastWriteWins(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.a"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.b"},
				Mode:  eventv1.ResponseFilterModeExclude,
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold", "silver"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Last scope is "silver" with Exclude, so mode should be Exclude
	if result.ResponseFilter.Mode != eventv1.ResponseFilterModeExclude {
		t.Errorf("expected mode Exclude (last-write-wins), got %q", result.ResponseFilter.Mode)
	}
}

func TestCreatePublisherTrigger_SelectionAndResponseCombined(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "premium"},
			},
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.name", "$.data.id"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SelectionFilter == nil || result.SelectionFilter.Expression == nil {
		t.Error("expected SelectionFilter to be set")
	}
	if result.ResponseFilter == nil {
		t.Error("expected ResponseFilter to be set")
	}
	if len(result.ResponseFilter.Paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(result.ResponseFilter.Paths))
	}
}

func TestCreatePublisherTrigger_ScopeWithoutSelectionFilter(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.name"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
			// No SelectionFilter
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.SelectionFilter != nil {
		t.Error("expected nil SelectionFilter when scope has no selection filter")
	}
	if result.ResponseFilter == nil {
		t.Fatal("expected ResponseFilter to be set")
	}
	if len(result.ResponseFilter.Paths) != 1 || result.ResponseFilter.Paths[0] != "$.data.name" {
		t.Errorf("unexpected paths: %v", result.ResponseFilter.Paths)
	}
}

func TestCreatePublisherTrigger_ScopeWithEmptyTrigger(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			// Both nil — nothing to contribute
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold"})

	// No selection filter, no response filter → should be nil
	if result != nil {
		t.Errorf("expected nil for scope with empty trigger, got: %+v", result)
	}
}

func TestCreatePublisherTrigger_OnlyMatchingScopes(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "A"},
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "B"},
			},
		}},
		{Name: "bronze", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "C"},
			},
		}},
	})

	// Only subscribe to "gold" — should NOT get OR wrapper since only 1 matching scope
	result := createPublisherTrigger(exposure, []string{"gold"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	// Should be direct eq, no "or" wrapper
	if _, hasOr := got["or"]; hasOr {
		t.Error("expected no 'or' wrapper for single matching scope")
	}
	eq := got["eq"].(map[string]any)
	if eq["value"] != "A" {
		t.Errorf("expected type=A from gold scope, got %v", eq["value"])
	}
}

func TestCreatePublisherTrigger_SubsetOfScopes(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "A"},
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "B"},
			},
		}},
		{Name: "bronze", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"type": "C"},
			},
		}},
	})

	// Subscribe to gold and bronze (skip silver)
	result := createPublisherTrigger(exposure, []string{"gold", "bronze"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	got := jsonToMap(t, result.SelectionFilter.Expression)

	orList, ok := got["or"].([]any)
	if !ok {
		t.Fatalf("expected 'or' for two matching scopes, got: %v", got)
	}
	if len(orList) != 2 {
		t.Fatalf("expected 2 items in 'or', got %d", len(orList))
	}

	// Verify values are A and C (gold and bronze), not B (silver)
	first := orList[0].(map[string]any)["eq"].(map[string]any)
	second := orList[1].(map[string]any)["eq"].(map[string]any)

	if first["value"] != "A" {
		t.Errorf("first scope: expected A (gold), got %v", first["value"])
	}
	if second["value"] != "C" {
		t.Errorf("second scope: expected C (bronze), got %v", second["value"])
	}
}

func TestCreatePublisherTrigger_SelectionFilterWithEmptyAttributes(t *testing.T) {
	// SelectionFilter is non-nil but has empty attributes and no expression
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{},
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold"})

	// Empty attributes → no expression generated → nil result
	if result != nil {
		t.Errorf("expected nil for scope with empty attributes, got: %+v", result)
	}
}

func TestCreatePublisherTrigger_ResponseFilterModeWithEmptyStringNotOverwritten(t *testing.T) {
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.a"},
				Mode:  eventv1.ResponseFilterModeExclude,
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.b"},
				Mode:  "", // Empty mode should not overwrite previous
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold", "silver"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Gold set Exclude, silver has empty mode → should stay Exclude
	if result.ResponseFilter.Mode != eventv1.ResponseFilterModeExclude {
		t.Errorf("expected mode Exclude (empty should not overwrite), got %q", result.ResponseFilter.Mode)
	}
}

// =========================================================================
// applyPublisherTrigger tests
// =========================================================================

func TestApplyPublisherTrigger_AccumulatesSelectionExpressions(t *testing.T) {
	result := &eventv1.EventTrigger{}
	var exprs []map[string]any

	trigger1 := &eventv1.EventTrigger{
		SelectionFilter: &eventv1.SelectionFilter{
			Attributes: map[string]string{"type": "A"},
		},
	}
	trigger2 := &eventv1.EventTrigger{
		SelectionFilter: &eventv1.SelectionFilter{
			Expression: mustJSON(t, map[string]any{"gt": map[string]any{"field": "x", "value": float64(1)}}),
		},
	}

	applyPublisherTrigger(trigger1, &exprs, result)
	applyPublisherTrigger(trigger2, &exprs, result)

	if len(exprs) != 2 {
		t.Fatalf("expected 2 accumulated expressions, got %d", len(exprs))
	}
}

func TestApplyPublisherTrigger_AccumulatesResponseFilterPaths(t *testing.T) {
	result := &eventv1.EventTrigger{}
	var exprs []map[string]any

	trigger1 := &eventv1.EventTrigger{
		ResponseFilter: &eventv1.ResponseFilter{
			Paths: []string{"a", "b"},
			Mode:  eventv1.ResponseFilterModeInclude,
		},
	}
	trigger2 := &eventv1.EventTrigger{
		ResponseFilter: &eventv1.ResponseFilter{
			Paths: []string{"c"},
			Mode:  eventv1.ResponseFilterModeExclude,
		},
	}

	applyPublisherTrigger(trigger1, &exprs, result)
	applyPublisherTrigger(trigger2, &exprs, result)

	if result.ResponseFilter == nil {
		t.Fatal("expected ResponseFilter to be set")
	}
	if len(result.ResponseFilter.Paths) != 3 {
		t.Errorf("expected 3 accumulated paths, got %d: %v", len(result.ResponseFilter.Paths), result.ResponseFilter.Paths)
	}
	// Mode should be Exclude (last write)
	if result.ResponseFilter.Mode != eventv1.ResponseFilterModeExclude {
		t.Errorf("expected mode Exclude, got %q", result.ResponseFilter.Mode)
	}
}

func TestApplyPublisherTrigger_SkipsNilFilters(t *testing.T) {
	result := &eventv1.EventTrigger{}
	var exprs []map[string]any

	trigger := &eventv1.EventTrigger{
		// Both nil
	}

	applyPublisherTrigger(trigger, &exprs, result)

	if len(exprs) != 0 {
		t.Errorf("expected 0 expressions for nil filter, got %d", len(exprs))
	}
	if result.ResponseFilter != nil {
		t.Errorf("expected nil ResponseFilter, got %+v", result.ResponseFilter)
	}
}

// =========================================================================
// Integration-style tests: three scopes with mixed filters
// =========================================================================

func TestCreatePublisherTrigger_ThreeScopes_FullMerge(t *testing.T) {
	customExpr := map[string]any{"ne": map[string]any{"field": "status", "value": "draft"}}
	exposure := makeExposure([]eventv1.EventScope{
		{Name: "gold", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"tier": "premium"},
			},
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.name", "$.data.id"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
		{Name: "silver", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Expression: mustJSON(t, customExpr),
			},
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.id", "$.data.email"},
				Mode:  eventv1.ResponseFilterModeExclude,
			},
		}},
		{Name: "bronze", Trigger: eventv1.EventTrigger{
			SelectionFilter: &eventv1.SelectionFilter{
				Attributes: map[string]string{"region": "eu", "priority": "low"},
			},
			ResponseFilter: &eventv1.ResponseFilter{
				Paths: []string{"$.data.name"},
				Mode:  eventv1.ResponseFilterModeInclude,
			},
		}},
	})

	result := createPublisherTrigger(exposure, []string{"gold", "silver", "bronze"})

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Check selection filter: 3 scopes → {"or": [gold_expr, silver_expr, bronze_expr]}
	got := jsonToMap(t, result.SelectionFilter.Expression)
	orList, ok := got["or"].([]any)
	if !ok {
		t.Fatalf("expected 'or' for 3 scopes, got: %v", got)
	}
	if len(orList) != 3 {
		t.Fatalf("expected 3 items in 'or', got %d", len(orList))
	}

	// Gold: {"eq": {"field":"tier","value":"premium"}} (single attribute)
	goldExpr := orList[0].(map[string]any)
	if _, hasEq := goldExpr["eq"]; !hasEq {
		t.Errorf("gold should have 'eq', got: %v", goldExpr)
	}

	// Silver: {"ne": ...} (pass-through expression)
	silverExpr := orList[1].(map[string]any)
	if _, hasNe := silverExpr["ne"]; !hasNe {
		t.Errorf("silver should have 'ne', got: %v", silverExpr)
	}

	// Bronze: {"and": [...]} (two attributes)
	bronzeExpr := orList[2].(map[string]any)
	if _, hasAnd := bronzeExpr["and"]; !hasAnd {
		t.Errorf("bronze should have 'and' for 2 attributes, got: %v", bronzeExpr)
	}

	// Check response filter: union of paths deduplicated
	if result.ResponseFilter == nil {
		t.Fatal("expected ResponseFilter to be set")
	}
	// Paths: [$.data.name, $.data.id] ∪ [$.data.id, $.data.email] ∪ [$.data.name] → [$.data.name, $.data.id, $.data.email]
	expectedPaths := []string{"$.data.name", "$.data.id", "$.data.email"}
	if len(result.ResponseFilter.Paths) != len(expectedPaths) {
		t.Fatalf("expected %d paths, got %d: %v", len(expectedPaths), len(result.ResponseFilter.Paths), result.ResponseFilter.Paths)
	}
	for i, p := range result.ResponseFilter.Paths {
		if p != expectedPaths[i] {
			t.Errorf("path[%d] = %q, want %q", i, p, expectedPaths[i])
		}
	}

	// Mode: last-write-wins → "bronze" is last → Include
	if result.ResponseFilter.Mode != eventv1.ResponseFilterModeInclude {
		t.Errorf("expected mode Include (last-write-wins from bronze), got %q", result.ResponseFilter.Mode)
	}
}
