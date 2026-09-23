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
	for _, exposureType := range []string{"api", "ai"} {
		Context(exposureType+" OAuth2 exposure", func() {
			var mapSecurity func(api.Oauth2) *roverv1.Security

			BeforeEach(func() {
				mapSecurity = func(oauth2 api.Oauth2) *roverv1.Security {
					security := api.Security{}
					Expect(security.FromOauth2(oauth2)).To(Succeed())
					if exposureType == "ai" {
						return mapAiExposure(api.AiExposure{Security: security}).Security
					}
					return mapApiExposure(api.ApiExposure{Security: security}).Security
				}
			})

			DescribeTable("maps ExternalIDP grant and token request settings",
				func(grant, tokenRequest string, expectedGrant roverv1.GrantType, expectedRequest roverv1.TokenRequestMethod) {
					security := mapSecurity(api.Oauth2{
						TokenEndpoint: "https://test.com/token",
						ClientId:      "client-id",
						ClientSecret:  "client-secret",
						GrantType:     api.GrantType(grant),
						TokenRequest:  api.Oauth2TokenRequest(tokenRequest),
					})

					Expect(security).ToNot(BeNil())
					Expect(security.M2M).ToNot(BeNil())
					Expect(security.M2M.ExternalIDP).ToNot(BeNil())
					idp := security.M2M.ExternalIDP
					Expect(idp.GrantType).To(Equal(expectedGrant))
					Expect(idp.TokenRequest).To(Equal(expectedRequest))
					Expect(idp.TokenEndpoint).To(Equal("https://test.com/token"))
					Expect(idp.Client).ToNot(BeNil())
					Expect(idp.Client.ClientId).To(Equal("client-id"))
					Expect(idp.Client.ClientSecret).To(Equal("client-secret"))
				},
				Entry("defaults both omitted fields", "", "", roverv1.GrantTypeClientCredentials, roverv1.TokenRequestClientSecretBasic),
				Entry("defaults only the omitted grant", "", "body", roverv1.GrantTypeClientCredentials, roverv1.TokenRequestClientSecretPost),
				Entry("defaults only the omitted token request", "PASSWORD", "", roverv1.GrantTypePassword, roverv1.TokenRequestClientSecretBasic),
				Entry("preserves lowercase settings", "client_credentials", "header", roverv1.GrantTypeClientCredentials, roverv1.TokenRequestClientSecretBasic),
				Entry("normalizes uppercase settings", "CLIENT_CREDENTIALS", "HEADER", roverv1.GrantTypeClientCredentials, roverv1.TokenRequestClientSecretBasic),
				Entry("preserves password and body", "password", "body", roverv1.GrantTypePassword, roverv1.TokenRequestClientSecretPost),
				Entry("normalizes refresh token and body", "REFRESH_TOKEN", "BODY", roverv1.GrantTypeRefreshToken, roverv1.TokenRequestClientSecretPost),
				Entry("preserves lowercase refresh token", "refresh_token", "header", roverv1.GrantTypeRefreshToken, roverv1.TokenRequestClientSecretBasic),
				Entry("normalizes the legacy basic alias", "PASSWORD", "BASIC", roverv1.GrantTypePassword, roverv1.TokenRequestClientSecretBasic),
				Entry("preserves canonical basic method", "client_credentials", "client_secret_basic", roverv1.GrantTypeClientCredentials, roverv1.TokenRequestClientSecretBasic),
				Entry("preserves canonical post method", "client_credentials", "client_secret_post", roverv1.GrantTypeClientCredentials, roverv1.TokenRequestClientSecretPost),
				Entry("keeps an invalid grant for validation", "INVALID", "", roverv1.GrantType("invalid"), roverv1.TokenRequestClientSecretBasic),
				Entry("keeps an invalid token request for validation", "", "INVALID", roverv1.GrantTypeClientCredentials, roverv1.TokenRequestMethod("INVALID")),
				Entry("does not default whitespace", " ", " ", roverv1.GrantType(" "), roverv1.TokenRequestMethod(" ")),
			)

			It("keeps scopes-only security on the platform identity provider", func() {
				security := mapSecurity(api.Oauth2{Scopes: []string{"read", "write"}})
				Expect(security).ToNot(BeNil())
				Expect(security.M2M).ToNot(BeNil())
				Expect(security.M2M.ExternalIDP).To(BeNil())
				Expect(security.M2M.Scopes).To(Equal([]string{"read", "write"}))
			})

			It("keeps empty OAuth2 security on the platform identity provider", func() {
				Expect(mapSecurity(api.Oauth2{})).To(BeNil())
			})
		})
	}

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
			Expect(output.Security.M2M.ExternalIDP).To(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		DescribeTable("must translate a symbolic valueFrom claim",
			func(apiValue api.ClaimValueFrom, domainValue roverv1.ClaimValueFrom) {
				// Given
				input := api.ApiExposure{BasePath: "/test"}
				oauth2 := api.Oauth2{
					Scopes: []string{"read"},
					Claims: api.Claims{Aud: api.Claim{ValueFrom: apiValue}},
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
				Expect(output.Security.M2M.Claims.Aud.ValueFrom).To(Equal(domainValue))
			},
			Entry("provider client ID", api.PROVIDERCLIENTID, roverv1.ClaimValueFromProviderClientId),
			Entry("consumer client ID", api.CONSUMERCLIENTID, roverv1.ClaimValueFromConsumerClientId),
			Entry("base path", api.BASEPATH, roverv1.ClaimValueFromBasePath),
		)

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
