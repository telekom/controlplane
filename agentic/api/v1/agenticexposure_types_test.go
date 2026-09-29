// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HasExternalIdp", func() {
	DescribeTable("requires a literal non-empty M2M token endpoint", func(security *Security, expected bool) {
		exposure := &AgenticExposure{Spec: AgenticExposureSpec{Security: security}}
		Expect(exposure.HasExternalIdp()).To(Equal(expected))
	},
		Entry("absent security", nil, false),
		Entry("absent M2M", &Security{}, false),
		Entry("absent IDP", &Security{M2M: &Machine2MachineAuthentication{}}, false),
		Entry("basic credentials", &Security{M2M: &Machine2MachineAuthentication{Basic: &BasicAuthCredentials{Username: "user", Password: "secret"}}}, false),
		Entry("empty endpoint", &Security{M2M: &Machine2MachineAuthentication{ExternalIDP: &ExternalIdentityProvider{}}}, false),
		Entry("populated endpoint", &Security{M2M: &Machine2MachineAuthentication{ExternalIDP: &ExternalIdentityProvider{TokenEndpoint: "https://idp.example/token"}}}, true),
		Entry("literal whitespace", &Security{M2M: &Machine2MachineAuthentication{ExternalIDP: &ExternalIdentityProvider{TokenEndpoint: " "}}}, true),
	)
})
