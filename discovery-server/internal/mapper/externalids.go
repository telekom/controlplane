// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
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
