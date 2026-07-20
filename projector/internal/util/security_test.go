// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/util"
)

func TestSecurity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Util Suite")
}

var _ = Describe("Credential mappers nil-safety", func() {
	It("MapCrBasicAuthToCpApi returns nil for nil input", func() {
		Expect(util.MapCrBasicAuthToCpApi(nil)).To(BeNil())
	})

	It("MapCrOAuthToCpApi returns nil for nil input", func() {
		Expect(util.MapCrOAuthToCpApi(nil)).To(BeNil())
	})

	It("MapCrExternalIdpToCpApi returns nil for nil input", func() {
		Expect(util.MapCrExternalIdpToCpApi(nil)).To(BeNil())
	})

	It("MapCrExternalIdpToCpApi maps nil nested creds to nil without panic", func() {
		idp := util.MapCrExternalIdpToCpApi(&apiv1.ExternalIdentityProvider{
			TokenEndpoint: "https://idp/token",
		})
		Expect(idp).NotTo(BeNil())
		Expect(idp.Basic).To(BeNil())
		Expect(idp.Client).To(BeNil())
	})
})
