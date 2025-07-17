// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import "slices"

// IsSubsetOfScopes checks if all requested scopes are present in the provided scopes.
// It returns all requested scopes that are not found in the provided scopes.
func IsSubsetOfScopes(providedScopes, requestedScopes []string) (bool, []string) {
	var invalidScopes []string

	for _, scope := range requestedScopes {
		if !slices.Contains(providedScopes, scope) {
			invalidScopes = append(invalidScopes, scope)
		}
	}
	if len(invalidScopes) > 0 {
		return false, invalidScopes
	}
	return true, nil
}
