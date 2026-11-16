// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"fmt"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
)

// ExternalId schemes exposed as scalar API fields. Must match the schemes
// written by rover-server (rover-server/internal/mapper/externalids.go).
const (
	PsiScheme  = "psi"
	IctoScheme = "icto"
)

// ExternalId returns the id for the given scheme, or "" if not present.
func ExternalId(ids []applicationv1.ExternalId, scheme string) string {
	for _, e := range ids {
		if e.Scheme == scheme {
			return e.Id
		}
	}
	return ""
}

// ExternalIdJsonPath returns the gjson path used in store.Filter to select the
// id of an Application's external id with the given scheme. Schemes are unique
// per Application (listMapKey=scheme), so the first match is the only one.
func ExternalIdJsonPath(scheme string) string {
	return fmt.Sprintf(`spec.externalIds.#(scheme==%q).id`, scheme)
}
