// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

// Scalar customer-facing fields map to internal ExternalId.Scheme values here.
// This is the single place where scalar names cross into scheme names; adding a
// new scalar-scheme pair is a one-line change.
const (
	PsiScheme  = "psi"
	IctoScheme = "icto"
)

// RoverScalarsToExternalIds packs non-empty customer scalar fields into a
// deterministic []ExternalId for the internal Rover CR. Returns nil when no
// scalars are supplied.
func RoverScalarsToExternalIds(psiid, icto string) []roverv1.ExternalId {
	var out []roverv1.ExternalId
	if psiid != "" {
		out = append(out, roverv1.ExternalId{Scheme: PsiScheme, Id: psiid})
	}
	if icto != "" {
		out = append(out, roverv1.ExternalId{Scheme: IctoScheme, Id: icto})
	}
	return out
}

// RoverExternalIdsToScalars projects a Rover's ExternalIds back onto the
// customer-facing scalar fields. Unknown schemes are ignored.
func RoverExternalIdsToScalars(ids []roverv1.ExternalId) (psiid, icto string) {
	for _, e := range ids {
		switch e.Scheme {
		case PsiScheme:
			psiid = e.Id
		case IctoScheme:
			icto = e.Id
		}
	}
	return
}

// ApplicationExternalIdsToScalars is the Application-CR counterpart to
// RoverExternalIdsToScalars. Kept separate because the Application API group
// has its own ExternalId type.
func ApplicationExternalIdsToScalars(ids []applicationv1.ExternalId) (psiid, icto string) {
	for _, e := range ids {
		switch e.Scheme {
		case PsiScheme:
			psiid = e.Id
		case IctoScheme:
			icto = e.Id
		}
	}
	return
}
