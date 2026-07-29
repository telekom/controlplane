// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Exposure Security Mapper", func() {
	Context("mapExposureSecurity", func() {
		It("must map BasicAuth security correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
			}
			basicAuth := api.BasicAuth{
				Username: "testuser",
				Password: "testpass",
			}
			input.Security = api.Security{}
			err := input.Security.FromBasicAuth(basicAuth)
			Expect(err).To(BeNil())

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Basic).ToNot(BeNil())
			Expect(output.Security.M2M.Basic.Username).To(Equal("testuser"))
			Expect(output.Security.M2M.Basic.Password).To(Equal("testpass"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must map OAuth2 ExternalIDP security correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
			}
			oauth2 := api.Oauth2{
				TokenEndpoint: "https://test.com/token",
				TokenRequest:  "basic",
				GrantType:     "client_credentials",
				ClientId:      "client-id",
				ClientSecret:  "client-secret",
				ClientKey:     "client-key",
				RefreshToken:  "refreshToken",
			}
			input.Security = api.Security{}
			err := input.Security.FromOauth2(oauth2)
			Expect(err).To(BeNil())

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.ExternalIDP).ToNot(BeNil())
			Expect(output.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://test.com/token"))
			Expect(output.Security.M2M.ExternalIDP.TokenRequest).To(Equal(roverv1.TokenRequestClientSecretBasic))
			Expect(output.Security.M2M.ExternalIDP.GrantType).To(Equal(roverv1.GrantTypeClientCredentials))
			Expect(output.Security.M2M.ExternalIDP.Client).ToNot(BeNil())
			Expect(output.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("client-id"))
			Expect(output.Security.M2M.ExternalIDP.Client.RefreshToken).To(Equal("refreshToken"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must map OAuth2 with username/password correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
			}
			oauth2 := api.Oauth2{
				TokenEndpoint: "https://test.com/token",
				TokenRequest:  "basic",
				GrantType:     "password",
				Username:      "testuser",
				Password:      "testpass",
			}
			input.Security = api.Security{}
			err := input.Security.FromOauth2(oauth2)
			Expect(err).To(BeNil())

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.ExternalIDP).ToNot(BeNil())
			Expect(output.Security.M2M.ExternalIDP.Basic).ToNot(BeNil())
			Expect(output.Security.M2M.ExternalIDP.Basic.Username).To(Equal("testuser"))
			Expect(output.Security.M2M.ExternalIDP.Basic.Password).To(Equal("testpass"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must map OAuth2 scopes correctly", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
			}
			oauth2 := api.Oauth2{
				Scopes: []string{"read", "write"},
			}
			input.Security = api.Security{}
			err := input.Security.FromOauth2(oauth2)
			Expect(err).To(BeNil())

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Scopes).To(ContainElements("read", "write"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must map OAuth2 claims-only (LMS default) correctly", func() {
			// Given
			input := api.ApiExposure{BasePath: "/test"}
			oauth2 := api.Oauth2{
				Claims: api.Claims{Aud: api.Claim{Value: "my-audience"}},
			}
			input.Security = api.Security{}
			Expect(input.Security.FromOauth2(oauth2)).To(Succeed())

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Claims).ToNot(BeNil())
			Expect(output.Security.M2M.Claims.Aud.Value).To(Equal("my-audience"))
			Expect(output.Security.M2M.Claims.Aud.ValueFrom).To(BeEmpty())
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must keep a symbolic valueFrom claim", func() {
			// Given
			input := api.ApiExposure{BasePath: "/test"}
			oauth2 := api.Oauth2{
				Scopes: []string{"read"},
				Claims: api.Claims{Aud: api.Claim{ValueFrom: api.ProviderClientId}},
			}
			input.Security = api.Security{}
			Expect(input.Security.FromOauth2(oauth2)).To(Succeed())

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M.Claims).ToNot(BeNil())
			Expect(output.Security.M2M.Claims.Aud.Value).To(BeEmpty())
			Expect(output.Security.M2M.Claims.Aud.ValueFrom).To(Equal(roverv1.ClaimValueFromProviderClientId))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must not set claims when aud is empty", func() {
			// Given
			input := api.ApiExposure{BasePath: "/test"}
			oauth2 := api.Oauth2{Scopes: []string{"read"}}
			input.Security = api.Security{}
			Expect(input.Security.FromOauth2(oauth2)).To(Succeed())

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M.Claims).To(BeNil())
		})

		It("must handle invalid security discriminator", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
			}
			input.Security = api.Security{} // Invalid security without proper initialization

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).To(BeNil()) // Should not set security when there's an error
		})

		It("must handle nil security", func() {
			// Given
			input := api.ApiExposure{
				BasePath: "/test",
			}
			// Security is not set

			output := &roverv1.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).To(BeNil()) // Should not set security when it's nil
		})
	})
})
