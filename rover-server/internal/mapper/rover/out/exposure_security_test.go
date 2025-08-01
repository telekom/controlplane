// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Exposure Security Mapper (Out)", func() {
	Context("mapExposureSecurity", func() {
		It("must map BasicAuth security correctly", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Security: &roverv1.Security{
					M2M: &roverv1.Machine2MachineAuthentication{
						Basic: &roverv1.BasicAuthCredentials{
							Username: "testuser",
							Password: "testpass",
						},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			basicAuth, err := output.Security.AsBasicAuth()
			Expect(err).To(BeNil())
			Expect(basicAuth.Username).To(Equal("testuser"))
			Expect(basicAuth.Password).To(Equal("testpass"))
			snaps.MatchSnapshot(GinkgoT(), basicAuth)
		})

		It("must map OAuth2 ExternalIDP security correctly", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Security: &roverv1.Security{
					M2M: &roverv1.Machine2MachineAuthentication{
						ExternalIDP: &roverv1.ExternalIdentityProvider{
							TokenEndpoint: "https://test.com/token",
							TokenRequest:  "basic",
							GrantType:     "client_credentials",
							Client: &roverv1.OAuth2ClientCredentials{
								ClientId:     "client-id",
								ClientSecret: "client-secret",
								ClientKey:    "client-key",
							},
						},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).To(BeNil())
			Expect(oauth2.TokenEndpoint).To(Equal("https://test.com/token"))
			Expect(oauth2.TokenRequest).To(Equal(api.Oauth2TokenRequest("basic")))
			Expect(oauth2.GrantType).To(Equal(api.GrantType("client_credentials")))
			Expect(oauth2.ClientId).To(Equal("client-id"))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})

		It("must map OAuth2 with username/password correctly", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Security: &roverv1.Security{
					M2M: &roverv1.Machine2MachineAuthentication{
						ExternalIDP: &roverv1.ExternalIdentityProvider{
							TokenEndpoint: "https://test.com/token",
							GrantType:     "password",
							Basic: &roverv1.BasicAuthCredentials{
								Username: "testuser",
								Password: "testpass",
							},
						},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).To(BeNil())
			Expect(oauth2.TokenEndpoint).To(Equal("https://test.com/token"))
			Expect(oauth2.GrantType).To(Equal(api.GrantType("password")))
			Expect(oauth2.Username).To(Equal("testuser"))
			Expect(oauth2.Password).To(Equal("testpass"))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})

		It("must map OAuth2 scopes correctly", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Security: &roverv1.Security{
					M2M: &roverv1.Machine2MachineAuthentication{
						Scopes: []string{"read", "write"},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).To(BeNil())
			Expect(oauth2.Scopes).To(ContainElements("read", "write"))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})

		It("must handle nil security", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				// Security is nil
			}

			output := &api.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).To(BeZero())
		})

		It("must handle nil M2M security", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Security: &roverv1.Security{
					// M2M is nil
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).To(BeZero())
		})

		It("must map combined scopes with ExternalIDP", func() {
			// Given
			input := &roverv1.ApiExposure{
				BasePath: "/test",
				Security: &roverv1.Security{
					M2M: &roverv1.Machine2MachineAuthentication{
						ExternalIDP: &roverv1.ExternalIdentityProvider{
							TokenEndpoint: "https://test.com/token",
						},
						Scopes: []string{"read", "write"},
					},
				},
			}

			output := &api.ApiExposure{}

			// When
			mapExposureSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).To(BeNil())
			Expect(oauth2.TokenEndpoint).To(Equal("https://test.com/token"))
			Expect(oauth2.Scopes).To(ContainElements("read", "write"))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})
	})
})
