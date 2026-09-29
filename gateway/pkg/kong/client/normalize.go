// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"cmp"
	"encoding/json"
	"slices"
)

// normalizeSet sorts and de-duplicates a set-like field, so that an ordering
// difference between the desired body and Kong's response is not mistaken for a
// change. Empty and absent both normalize to absent.
func normalizeSet[T cmp.Ordered](values *[]T) *[]T {
	if values == nil || len(*values) == 0 {
		return nil
	}
	normalized := slices.Clone(*values)
	slices.Sort(normalized)
	normalized = slices.Compact(normalized)
	return &normalized
}

// convertIntSet normalizes a set and converts it to the named element type the
// generated request bodies use.
func convertIntSet[Out ~int](values *[]int) *[]Out {
	normalized := normalizeSet(values)
	if normalized == nil {
		return nil
	}
	converted := make([]Out, len(*normalized))
	for i, value := range *normalized {
		converted[i] = Out(value)
	}
	return &converted
}

// convertStringSet normalizes a set and converts it to the named element type
// the generated request bodies use.
func convertStringSet[Out ~string](values *[]string) *[]Out {
	normalized := normalizeSet(values)
	if normalized == nil {
		return nil
	}
	converted := make([]Out, len(*normalized))
	for i, value := range *normalized {
		converted[i] = Out(value)
	}
	return &converted
}

func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T
		return zero
	}
	return *value
}

// normalizeConfig converts a plugin configuration into the value types Kong
// returns: the custom set types the features build become arrays and integers
// become float64. Without this the desired configuration could never equal one
// read back from Kong. The round trip also copies the configuration, so
// sorting it does not mutate the caller's map.
func normalizeConfig(config map[string]any) (map[string]any, error) {
	if config == nil {
		return nil, nil
	}
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	normalized := make(map[string]any)
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}
	sortConfigSets(normalized)
	return normalized, nil
}

// narrowToDesired returns the part of current that desired names. Kong answers
// with every plugin configuration key filled in from its schema defaults, while
// the desired configuration holds only the keys a feature sets, so comparing
// the whole answer would report a difference on every reconciliation and write
// the plugin every time. Nesting is followed so that defaults inside a group -
// for example the unset members of a rate-limiting policy - are dropped too.
//
// A key the desired configuration names but Kong does not report is left out,
// so it still counts as a difference and the plugin is written.
func narrowToDesired(desired, current map[string]any) map[string]any {
	if desired == nil {
		return nil
	}
	narrowed := make(map[string]any, len(desired))
	for key, desiredValue := range desired {
		currentValue, found := current[key]
		if !found {
			continue
		}
		desiredGroup, desiredIsGroup := desiredValue.(map[string]any)
		currentGroup, currentIsGroup := currentValue.(map[string]any)
		if desiredIsGroup && currentIsGroup {
			narrowed[key] = narrowToDesired(desiredGroup, currentGroup)
			continue
		}
		narrowed[key] = currentValue
	}
	return narrowed
}

// sortConfigSets sorts string arrays in a plugin configuration in place. Every
// array in the configurations this controller manages is a set: ACL allow and
// deny lists, allowed issuers, IP ranges, and request-transformer header lists.
// They are built from set types that marshal in Go map iteration order, which
// is randomized, so without sorting the desired and current configurations
// differ on nearly every reconciliation and Kong is written every time.
// Only arrays that hold exclusively strings are sorted, so an ordered array
// added to a future plugin configuration is left untouched.
//
// A configuration read from Kong is decoded with encoding/json and therefore
// already has canonical value types; only its ordering needs this treatment.
func sortConfigSets(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, item := range typed {
			sortConfigSets(item)
		}
	case []any:
		for _, item := range typed {
			sortConfigSets(item)
		}
		sortIfAllStrings(typed)
	}
}

func sortIfAllStrings(items []any) {
	sorted := make([]string, len(items))
	for i, item := range items {
		value, isString := item.(string)
		if !isString {
			return
		}
		sorted[i] = value
	}
	slices.Sort(sorted)
	for i, value := range sorted {
		items[i] = value
	}
}
