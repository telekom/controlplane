// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiapi "github.com/telekom/controlplane/api/api/v1"
)

var _ = Describe("Gateway Util", func() {
	Context("HasExternalIDP", func() {
		It("should return true if ExternalIDPConfig exists", func() {

			exposure := &apiapi.ApiExposure{}
			Expect(exposure.Spec.HasExternalIdp()).To(BeFalse())

			exposure.Spec = apiapi.ApiExposureSpec{}
			Expect(exposure.Spec.HasExternalIdp()).To(BeFalse())

			exposure.Spec.Security = &apiapi.Security{}
			Expect(exposure.Spec.HasExternalIdp()).To(BeFalse())

			exposure.Spec.Security.M2M = &apiapi.Machine2MachineAuthentication{}
			Expect(exposure.Spec.HasExternalIdp()).To(BeFalse())

			exposure.Spec.Security.M2M.ExternalIDP = &apiapi.ExternalIdentityProvider{}
			Expect(exposure.Spec.HasExternalIdp()).To(BeFalse())

			exposure.Spec.Security.M2M.ExternalIDP.TokenEndpoint = "https://example.com/token"
			Expect(exposure.Spec.HasExternalIdp()).To(BeTrue())
		})
	})
})
