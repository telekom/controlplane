// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package api

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiapi "github.com/telekom/controlplane/api/api/v1"
	rover "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("mapClaimsToApiClaims (exposure)", func() {
	const providerClientId = "eni--foo--api-provider-rover"
	const basePath = "/eni/foo/v1"

	It("returns nil for nil claims", func() {
		Expect(mapClaimsToApiClaims(nil, providerClientId, basePath)).To(BeNil())
	})

	It("returns nil when aud is unset", func() {
		Expect(mapClaimsToApiClaims(&rover.Claims{}, providerClientId, basePath)).To(BeNil())
	})

	It("copies a literal value through", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{Value: "my-audience"}}, providerClientId, basePath)
		Expect(got.Aud.Value).To(Equal("my-audience"))
		Expect(got.Aud.ValueFrom).To(BeEmpty())
	})

	It("resolves ProviderClientId to a literal", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromProviderClientId}}, providerClientId, basePath)
		Expect(got.Aud.Value).To(Equal(providerClientId))
		Expect(got.Aud.ValueFrom).To(BeEmpty())
	})

	It("resolves BasePath to a literal", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromBasePath}}, providerClientId, basePath)
		Expect(got.Aud.Value).To(Equal(basePath))
	})

	It("keeps ConsumerClientId symbolic", func() {
		got := mapClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromConsumerClientId}}, providerClientId, basePath)
		Expect(got.Aud.Value).To(BeEmpty())
		Expect(got.Aud.ValueFrom).To(Equal(apiapi.ClaimValueFromConsumerClientId))
	})
})

var _ = Describe("mapSubscriberClaimsToApiClaims (subscription)", func() {
	It("returns nil for nil claims", func() {
		Expect(mapSubscriberClaimsToApiClaims(nil)).To(BeNil())
	})

	It("copies a literal value through", func() {
		got := mapSubscriberClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{Value: "consumer-audience"}})
		Expect(got.Aud.Value).To(Equal("consumer-audience"))
	})

	It("keeps ConsumerClientId symbolic", func() {
		got := mapSubscriberClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromConsumerClientId}})
		Expect(got.Aud.ValueFrom).To(Equal(apiapi.ClaimValueFromConsumerClientId))
	})

	It("ignores ProviderClientId (not resolvable on subscriber side)", func() {
		Expect(mapSubscriberClaimsToApiClaims(&rover.Claims{Aud: &rover.Claim{ValueFrom: rover.ClaimValueFromProviderClientId}})).To(BeNil())
	})
})
