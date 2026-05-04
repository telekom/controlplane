// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func TestRoverScalarsToExternalIds(t *testing.T) {
	cases := []struct {
		name  string
		psiid string
		icto  string
		want  []roverv1.ExternalId
	}{
		{"both empty", "", "", nil},
		{"only psiid", "PSI-103596", "", []roverv1.ExternalId{{Scheme: "psi", Id: "PSI-103596"}}},
		{"only icto", "", "icto-12345", []roverv1.ExternalId{{Scheme: "icto", Id: "icto-12345"}}},
		{"both", "PSI-103596", "icto-12345", []roverv1.ExternalId{
			{Scheme: "psi", Id: "PSI-103596"},
			{Scheme: "icto", Id: "icto-12345"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapper.RoverScalarsToExternalIds(tc.psiid, tc.icto)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRoverExternalIdsToScalars(t *testing.T) {
	ids := []roverv1.ExternalId{
		{Scheme: "psi", Id: "PSI-103596"},
		{Scheme: "icto", Id: "icto-12345"},
		{Scheme: "unknown", Id: "ignored"},
	}
	psiid, icto := mapper.RoverExternalIdsToScalars(ids)
	assert.Equal(t, "PSI-103596", psiid)
	assert.Equal(t, "icto-12345", icto)

	// Missing schemes return empty strings.
	psiid, icto = mapper.RoverExternalIdsToScalars(nil)
	assert.Equal(t, "", psiid)
	assert.Equal(t, "", icto)
}

func TestApplicationExternalIdsToScalars(t *testing.T) {
	ids := []applicationv1.ExternalId{
		{Scheme: "psi", Id: "PSI-103596"},
		{Scheme: "icto", Id: "icto-12345"},
	}
	psiid, icto := mapper.ApplicationExternalIdsToScalars(ids)
	assert.Equal(t, "PSI-103596", psiid)
	assert.Equal(t, "icto-12345", icto)
}
