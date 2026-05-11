// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/telekom/controlplane/rover-server/internal/mapper"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
)

func TestRoverScalarsToExternalIds(t *testing.T) {
	cases := []struct {
		name string
		in   mapper.ExternalIdScalars
		want []roverv1.ExternalId
	}{
		{"both empty", mapper.ExternalIdScalars{}, nil},
		{"only psiid", mapper.ExternalIdScalars{Psiid: "PSI-103596"}, []roverv1.ExternalId{{Scheme: "psi", Id: "PSI-103596"}}},
		{"only icto", mapper.ExternalIdScalars{Icto: "icto-12345"}, []roverv1.ExternalId{{Scheme: "icto", Id: "icto-12345"}}},
		{"both", mapper.ExternalIdScalars{Psiid: "PSI-103596", Icto: "icto-12345"}, []roverv1.ExternalId{
			{Scheme: "psi", Id: "PSI-103596"},
			{Scheme: "icto", Id: "icto-12345"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapper.RoverScalarsToExternalIds(tc.in)
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
	got := mapper.RoverExternalIdsToScalars(ids)
	assert.Equal(t, mapper.ExternalIdScalars{Psiid: "PSI-103596", Icto: "icto-12345"}, got)

	// Missing schemes yield the zero value.
	assert.Equal(t, mapper.ExternalIdScalars{}, mapper.RoverExternalIdsToScalars(nil))
}

