// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mapper

import (
	"testing"

	"github.com/onsi/gomega"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
)

func TestExternalId(t *testing.T) {
	t.Parallel()

	ids := []applicationv1.ExternalId{
		{Scheme: IctoScheme, Id: "icto-12345"},
		{Scheme: PsiScheme, Id: "PSI-123456"},
	}

	tests := []struct {
		name   string
		ids    []applicationv1.ExternalId
		scheme string
		expect string
	}{
		{name: "icto present", ids: ids, scheme: IctoScheme, expect: "icto-12345"},
		{name: "psi present", ids: ids, scheme: PsiScheme, expect: "PSI-123456"},
		{name: "unknown scheme", ids: ids, scheme: "other", expect: ""},
		{name: "nil ids", ids: nil, scheme: IctoScheme, expect: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := gomega.NewWithT(t)
			g.Expect(ExternalId(tt.ids, tt.scheme)).To(gomega.Equal(tt.expect))
		})
	}
}
