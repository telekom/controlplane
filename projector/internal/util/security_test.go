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
	It("MapCRBasicAuthToCPAPI returns nil for nil input", func() {
		Expect(util.MapCRBasicAuthToCPAPI(nil)).To(BeNil())
	})

	It("MapCROAuthToCPAPI returns nil for nil input", func() {
		Expect(util.MapCROAuthToCPAPI(nil)).To(BeNil())
	})

	It("MapCRExternalIDPToCPAPI returns nil for nil input", func() {
		Expect(util.MapCRExternalIDPToCPAPI(nil)).To(BeNil())
	})

	It("MapCRExternalIDPToCPAPI maps nil nested creds to nil without panic", func() {
		idp := util.MapCRExternalIDPToCPAPI(&apiv1.ExternalIdentityProvider{
			TokenEndpoint: "https://idp/token",
		})
		Expect(idp).NotTo(BeNil())
		Expect(idp.Basic).To(BeNil())
		Expect(idp.Client).To(BeNil())
	})
})
