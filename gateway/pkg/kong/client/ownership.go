// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"slices"
	"strings"
)

// legacyTagPrefix is the prefix of every tag the Java operator stamped on its entities.
const legacyTagPrefix = "ei__telekom__de--"

// ownership is a predicate over the tags of a Kong entity that proves it is ours.
type ownership func(tags []string) bool

// hasTag is satisfied by entities that carry the tag.
func hasTag(tag string) ownership {
	return func(tags []string) bool { return slices.Contains(tags, tag) }
}

// hasTagPrefix is satisfied by entities that carry a tag with the prefix.
func hasTagPrefix(prefix string) ownership {
	return func(tags []string) bool {
		return slices.ContainsFunc(tags, func(tag string) bool { return strings.HasPrefix(tag, prefix) })
	}
}

// allOf is satisfied if every predicate is.
func allOf(predicates ...ownership) ownership {
	return func(tags []string) bool {
		for _, p := range predicates {
			if !p(tags) {
				return false
			}
		}
		return true
	}
}

// anyOf is satisfied if at least one predicate is.
func anyOf(predicates ...ownership) ownership {
	return func(tags []string) bool {
		for _, p := range predicates {
			if p(tags) {
				return true
			}
		}
		return false
	}
}

// cpTags is satisfied by entities the control plane created for the route in the environment.
func cpTags(env, route string) ownership {
	return allOf(hasTag(BuildTag("env", env)), hasTag(BuildTag(parentRoute, route)))
}

// anyTag is satisfied by entities that carry at least one of the tags.
func anyTag(tags []string) ownership {
	predicates := make([]ownership, 0, len(tags))
	for _, tag := range tags {
		predicates = append(predicates, hasTag(tag))
	}
	return anyOf(predicates...)
}

// legacyTags returns the tags of the Java operator.
func legacyTags(tags []string) []string {
	var kept []string
	for _, tag := range tags {
		if strings.HasPrefix(tag, legacyTagPrefix) {
			kept = append(kept, tag)
		}
	}
	return kept
}

// notClaimedElsewhere is satisfied unless the entity carries an env-- or route-- tag of
// another environment or route. Entities without CP tags satisfy it.
func notClaimedElsewhere(env, route string) ownership {
	claims := map[string]string{BuildTag("env", ""): env, BuildTag(parentRoute, ""): route}
	return func(tags []string) bool {
		for _, tag := range tags {
			for prefix, own := range claims {
				if value, ok := strings.CutPrefix(tag, prefix); ok && value != own {
					return false
				}
			}
		}
		return true
	}
}
