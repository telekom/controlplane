// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription

import (
	"encoding/json"
	"slices"
	"sort"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// createPublisherTrigger merges publisher-defined triggers from all matching
// scopes into a single EventTrigger for the pubsub Subscriber.
//
// Merge strategy:
//   - SelectionFilter: Each scope's filter is converted to an expression and
//     combined with OR semantics: {"or": [scope1_expr, scope2_expr, ...]}
//     Single scope: expression used directly (no OR wrapper).
//   - ResponseFilter.Paths: Union of all paths, deduplicated.
//   - ResponseFilter.Mode: Last-write-wins (consistent with Java Horizon).
func createPublisherTrigger(exposure *eventv1.EventExposure, subscribedScopes []string) *eventv1.EventTrigger {
	if len(exposure.Spec.Scopes) == 0 || len(subscribedScopes) == 0 {
		return nil
	}

	result := &eventv1.EventTrigger{}
	var selectionExprs []map[string]any

	for _, eeScope := range exposure.Spec.Scopes {
		if slices.Contains(subscribedScopes, eeScope.Name) {
			applyPublisherTrigger(&eeScope.Trigger, &selectionExprs, result)
		}
	}

	finalizeSelectionFilter(result, selectionExprs)
	deduplicateResponseFilterPaths(result)

	if result.ResponseFilter == nil && result.SelectionFilter == nil {
		return nil
	}
	return result
}

// applyPublisherTrigger accumulates a single scope's trigger into the result.
// Selection filters are collected as expression maps for later OR-wrapping.
// Response filter paths are accumulated. Mode uses last-write-wins.
func applyPublisherTrigger(
	scopeTrigger *eventv1.EventTrigger,
	selectionExprs *[]map[string]any,
	result *eventv1.EventTrigger,
) {
	// Accumulate selection filters for OR-wrapping
	if scopeTrigger.SelectionFilter != nil {
		if scopeTrigger.SelectionFilter.Expression != nil {
			// Advanced selection filter: add the expression as-is
			var expr map[string]any
			if err := json.Unmarshal(scopeTrigger.SelectionFilter.Expression.Raw, &expr); err == nil {
				*selectionExprs = append(*selectionExprs, expr)
			}
		} else if len(scopeTrigger.SelectionFilter.Attributes) > 0 {
			// Simple selection filter: convert attributes to expression tree
			*selectionExprs = append(*selectionExprs,
				attributesToExpression(scopeTrigger.SelectionFilter.Attributes))
		}
	}

	// Accumulate response filter paths, last-write-wins for mode
	if scopeTrigger.ResponseFilter != nil {
		if result.ResponseFilter == nil {
			result.ResponseFilter = &eventv1.ResponseFilter{}
		}
		result.ResponseFilter.Paths = append(result.ResponseFilter.Paths, scopeTrigger.ResponseFilter.Paths...)
		if scopeTrigger.ResponseFilter.Mode != "" {
			result.ResponseFilter.Mode = scopeTrigger.ResponseFilter.Mode
		}
	}
}

// attributesToExpression converts a simple key-value attribute map to an expression tree.
//
// Single attribute:
//
//	{"color": "red"} → {"eq": {"field": "color", "value": "red"}}
//
// Multiple attributes (AND-ed):
//
//	{"color": "red", "size": "large"} → {"and": [{"eq": {"field": "color", "value": "red"}}, {"eq": {"field": "size", "value": "large"}}]}
func attributesToExpression(attrs map[string]string) map[string]any {
	eqExprs := make([]any, 0, len(attrs))

	// Sort keys for deterministic output
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		eqExprs = append(eqExprs, map[string]any{
			"eq": map[string]any{
				"field": k,
				"value": attrs[k],
			},
		})
	}

	if len(eqExprs) == 1 {
		return eqExprs[0].(map[string]any)
	}
	return map[string]any{"and": eqExprs}
}

// finalizeSelectionFilter wraps collected expression filters and sets them
// on the result's SelectionFilter.Expression.
//
//   - 0 expressions: no-op (no selection filter set)
//   - 1 expression: used directly (no OR wrapper)
//   - N expressions: wrapped in {"or": [expr1, expr2, ...]}
func finalizeSelectionFilter(result *eventv1.EventTrigger, exprs []map[string]any) {
	if len(exprs) == 0 {
		return
	}

	var exprMap map[string]any
	if len(exprs) == 1 {
		exprMap = exprs[0]
	} else {
		orList := make([]any, len(exprs))
		for i, e := range exprs {
			orList[i] = e
		}
		exprMap = map[string]any{"or": orList}
	}

	raw, err := json.Marshal(exprMap)
	if err != nil {
		return
	}

	result.SelectionFilter = &eventv1.SelectionFilter{
		Expression: &apiextensionsv1.JSON{Raw: raw},
	}
}

// deduplicateResponseFilterPaths removes duplicate paths from the response filter
// while preserving order.
func deduplicateResponseFilterPaths(result *eventv1.EventTrigger) {
	if result.ResponseFilter == nil || len(result.ResponseFilter.Paths) == 0 {
		return
	}

	seen := make(map[string]struct{})
	unique := make([]string, 0, len(result.ResponseFilter.Paths))
	for _, p := range result.ResponseFilter.Paths {
		if _, exists := seen[p]; !exists {
			seen[p] = struct{}{}
			unique = append(unique, p)
		}
	}
	result.ResponseFilter.Paths = unique
}
