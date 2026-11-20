// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	anyKey   = ".*"
	anyIndex = "[*]"
)

// parsePath splits a JSON path such as "$.config.limits[0]" into the segments
// ".config", ".limits" and "[0]". It accepts the wildcards ".*" and "[*]".
func parsePath(path string) ([]string, error) {
	rest, ok := strings.CutPrefix(path, Root)
	if !ok {
		return nil, fmt.Errorf("path %q must start with %q", path, Root)
	}
	var segments []string
	for rest != "" {
		switch rest[0] {
		case '.':
			end := strings.IndexAny(rest[1:], ".[")
			if end < 0 {
				end = len(rest) - 1
			}
			key := rest[1 : end+1]
			if key == "" {
				return nil, fmt.Errorf("path %q has an empty key", path)
			}
			segments = append(segments, "."+key)
			rest = rest[end+1:]
		case '[':
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("path %q has an unclosed index", path)
			}
			index := rest[1:end]
			if index != "*" {
				if _, err := strconv.Atoi(index); err != nil {
					return nil, fmt.Errorf("path %q has an invalid index %q", path, index)
				}
			}
			segments = append(segments, rest[:end+1])
			rest = rest[end+1:]
		default:
			return nil, fmt.Errorf("path %q has an unexpected %q", path, rest[0])
		}
	}
	return segments, nil
}

// covers reports whether pattern matches path or one of its ancestors.
func covers(pattern, path []string) bool {
	if len(pattern) > len(path) {
		return false
	}
	for i, p := range pattern {
		switch {
		case p == path[i]:
		case p == anyKey && strings.HasPrefix(path[i], "."):
		case p == anyIndex && strings.HasPrefix(path[i], "["):
		default:
			return false
		}
	}
	return true
}
