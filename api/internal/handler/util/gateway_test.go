// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
)

var _ = Describe("Gateway Util", func() {
	Context("HasExternalIDP", func() {
		It("should return true if ExternalIDP exists", func() {

			exposure := &apiapi.ApiExposure{}
			Expect(HasExternalIdp(exposure)).To(BeFalse())

			exposure.Spec = apiapi.ApiExposureSpec{}
			Expect(HasExternalIdp(exposure)).To(BeFalse())

			exposure.Spec.Security = &apiapi.Security{}
			Expect(HasExternalIdp(exposure)).To(BeFalse())

			exposure.Spec.Security.M2M = &apiapi.Machine2MachineAuthentication{}
			Expect(HasExternalIdp(exposure)).To(BeFalse())

			exposure.Spec.Security.M2M.ExternalIDP = &apiapi.ExternalIdentityProvider{}
			Expect(HasExternalIdp(exposure)).To(BeFalse())

			exposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint = "https://example.com/token"
			Expect(HasExternalIdp(exposure)).To(BeTrue())
		})
	})
})
