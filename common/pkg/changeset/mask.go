// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// maskPrefix starts every masked value.
const maskPrefix = "sha256:"

// Masker replaces the secret values in a canonical JSON body (maps, slices, strings,
// json.Numbers and bools) and returns the body. A secret value is replaced with the same
// mask on both sides of a comparison, so a changed secret is still a difference.
type Masker func(body any) any

// MaskPathsOrDie returns a Masker for the values at JSON paths such as "$.secret" or
// "$.config.*.password". An invalid path is a developer error and panics.
func MaskPathsOrDie(paths ...string) Masker {
	patterns := parsePatternsOrDie(paths)
	return func(body any) any {
		for _, pattern := range patterns {
			body = apply(body, pattern, func(v any) any { return mask(v) })
		}
		return body
	}
}

// MaskEntriesOrDie returns a Masker for "key:value" strings in the lists at path whose key
// is one of keys, such as the jumper_config entries of Kong request headers. The key stays
// visible; only the value is masked. An invalid path is a developer error and panics.
func MaskEntriesOrDie(path string, keys ...string) Masker {
	patterns := parsePatternsOrDie([]string{path})
	return func(body any) any {
		return apply(body, patterns[0], func(v any) any {
			list, ok := v.([]any)
			if !ok {
				return v
			}
			for i, item := range list {
				entry, ok := item.(string)
				if !ok {
					continue
				}
				key, value, found := strings.Cut(entry, ":")
				if found && slices.Contains(keys, key) {
					list[i] = key + ":" + mask(value)
				}
			}
			return list
		})
	}
}

func parsePatternsOrDie(paths []string) [][]string {
	patterns := make([][]string, 0, len(paths))
	for _, p := range paths {
		segments, err := parsePath(p)
		if err != nil {
			panic(fmt.Sprintf("invalid secret path: %v", err))
		}
		patterns = append(patterns, segments)
	}
	return patterns
}

// mask returns the masked form of a secret value.
func mask(v any) string {
	raw, ok := v.(string)
	if !ok {
		encoded, err := json.Marshal(v)
		if err != nil {
			encoded = []byte(fmt.Sprint(v))
		}
		raw = string(encoded)
	}
	sum := sha256.Sum256([]byte(raw))
	return maskPrefix + hex.EncodeToString(sum[:])[:hashLength]
}

// apply calls f on every value of node at the pattern and stores the result.
func apply(node any, pattern []string, f func(any) any) any {
	if node == nil {
		return nil
	}
	if len(pattern) == 0 {
		return f(node)
	}
	segment, rest := pattern[0], pattern[1:]
	switch {
	case strings.HasPrefix(segment, "."):
		fields, ok := node.(map[string]any)
		if !ok {
			return node
		}
		key := segment[1:]
		for k, child := range fields {
			if key == "*" || k == key {
				fields[k] = apply(child, rest, f)
			}
		}
		return fields
	default:
		items, ok := node.([]any)
		if !ok {
			return node
		}
		index := strings.Trim(segment, "[]")
		for i, child := range items {
			if index == "*" || index == strconv.Itoa(i) {
				items[i] = apply(child, rest, f)
			}
		}
		return items
	}
}
