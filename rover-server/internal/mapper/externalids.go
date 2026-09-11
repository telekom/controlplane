// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// Scalar customer-facing fields map to internal ExternalId.Scheme values here.
// This is the single place where scalar names cross into scheme names; adding a
// new scalar-scheme pair is a one-line change.
const (
	PsiScheme  = "psi"
	IctoScheme = "icto"
)

// ExternalIdScalars groups the customer-facing scalar identifier fields.
// Using a struct avoids mix-ups between positional string parameters that all
// have the same type.
type ExternalIdScalars struct {
	Psiid string
	Icto  string
}

// RoverScalarsToExternalIds packs non-empty customer scalar fields into a
// deterministic []ExternalId for the internal Rover CR. Returns nil when no
// scalars are supplied.
func RoverScalarsToExternalIds(scalars ExternalIdScalars) []roverv1.ExternalId {
	var out []roverv1.ExternalId
	if scalars.Psiid != "" {
		out = append(out, roverv1.ExternalId{Scheme: PsiScheme, Id: scalars.Psiid})
	}
	if scalars.Icto != "" {
		out = append(out, roverv1.ExternalId{Scheme: IctoScheme, Id: scalars.Icto})
	}
	return out
}

// RoverExternalIdsToScalars projects a Rover's ExternalIds back onto the
// customer-facing scalar fields. Unknown schemes are ignored.
func RoverExternalIdsToScalars(ids []roverv1.ExternalId) ExternalIdScalars {
	var out ExternalIdScalars
	for _, e := range ids {
		switch e.Scheme {
		case PsiScheme:
			out.Psiid = e.Id
		case IctoScheme:
			out.Icto = e.Id
		}
	}
	return out
}
