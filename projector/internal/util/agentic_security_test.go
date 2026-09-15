// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/projector/internal/util"
)

// Uses the shared Ginkgo suite entry point registered in security_test.go
// (TestSecurity / RunSpecs) — only one suite bootstrap is needed per package.
var _ = Describe("Agentic credential mappers nil-safety", func() {
	It("MapAgenticBasicAuthToCpApi returns nil for nil input", func() {
		Expect(util.MapAgenticBasicAuthToCpApi(nil)).To(BeNil())
	})

	It("MapAgenticOAuthToCpApi returns nil for nil input", func() {
		Expect(util.MapAgenticOAuthToCpApi(nil)).To(BeNil())
	})

	It("MapAgenticExternalIdpToCpApi returns nil for nil input", func() {
		Expect(util.MapAgenticExternalIdpToCpApi(nil)).To(BeNil())
	})

	It("MapAgenticExternalIdpToCpApi maps nil nested creds to nil without panic", func() {
		idp := util.MapAgenticExternalIdpToCpApi(&agenticv1.ExternalIdentityProvider{
			TokenEndpoint: "https://idp/token",
		})
		Expect(idp).NotTo(BeNil())
		Expect(idp.Basic).To(BeNil())
		Expect(idp.Client).To(BeNil())
	})
})
