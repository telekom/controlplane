// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
)

// Root is the JSON path of a whole entity. A diff at Root means the entity is created or deleted.
const Root = "$"

// Diff is one difference between the desired and the current state of a Kong entity.
// Desired is nil for a value only Kong has, Current is nil for a value only the
// operator wants.
type Diff struct {
	Path    string `json:"path"`
	Desired any    `json:"desired,omitempty"`
	Current any    `json:"current,omitempty"`
	// Ignored is true if the diff is on the ignore list.
	Ignored bool `json:"ignored,omitempty"`
}

// Compare returns the differences between desired and current, both serialized to
// JSON first. Null values, empty arrays and empty objects count as absent, because Kong
// and the operator use them interchangeably. Arrays of different length differ as a whole.
func Compare(desired, current any) ([]Diff, error) {
	d, err := canonical(desired)
	if err != nil {
		return nil, fmt.Errorf("serializing desired state: %w", err)
	}
	c, err := canonical(current)
	if err != nil {
		return nil, fmt.Errorf("serializing current state: %w", err)
	}
	var diffs []Diff
	walk(Root, d, c, &diffs)
	return diffs, nil
}

// canonical serializes v to JSON and decodes it into maps, slices and json.Numbers
// without absent-like values.
func canonical(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return nil, err
	}
	return prune(tree), nil
}

// prune removes null values, empty arrays and empty objects recursively. It returns nil
// for a value that is absent as a whole.
func prune(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if pruned := prune(child); pruned == nil {
				delete(t, k)
			} else {
				t[k] = pruned
			}
		}
		if len(t) == 0 {
			return nil
		}
		return t
	case []any:
		for i := range t {
			t[i] = prune(t[i])
		}
		if len(t) == 0 {
			return nil
		}
		return t
	default:
		return v
	}
}

func walk(path string, desired, current any, diffs *[]Diff) {
	dm, dIsMap := desired.(map[string]any)
	cm, cIsMap := current.(map[string]any)
	// An absent object is compared as an empty one, so that every field is reported by its own path.
	if dIsMap && current == nil {
		cm, cIsMap = map[string]any{}, true
	}
	if cIsMap && desired == nil {
		dm, dIsMap = map[string]any{}, true
	}
	if dIsMap && cIsMap {
		keys := make([]string, 0, len(dm)+len(cm))
		for k := range dm {
			keys = append(keys, k)
		}
		for k := range cm {
			if _, ok := dm[k]; !ok {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		for _, k := range keys {
			walk(path+"."+k, dm[k], cm[k], diffs)
		}
		return
	}

	da, dIsArray := desired.([]any)
	ca, cIsArray := current.([]any)
	if dIsArray && cIsArray && len(da) == len(ca) {
		for i := range da {
			walk(path+"["+strconv.Itoa(i)+"]", da[i], ca[i], diffs)
		}
		return
	}

	if !reflect.DeepEqual(desired, current) {
		*diffs = append(*diffs, Diff{Path: path, Desired: desired, Current: current})
	}
}

// nameFields are the fields that name a Kong entity, in lookup order: routes, services,
// upstreams and plugins have a name, targets a target and consumers a username.
var nameFields = []string{"name", "target", "username"}

// NameOf returns the name of a Kong entity body, or "" if it has none.
func NameOf(body any) string {
	tree, err := canonical(body)
	if err != nil {
		return ""
	}
	fields, ok := tree.(map[string]any)
	if !ok {
		return ""
	}
	for _, field := range nameFields {
		if name, ok := fields[field].(string); ok {
			return name
		}
	}
	return ""
}
