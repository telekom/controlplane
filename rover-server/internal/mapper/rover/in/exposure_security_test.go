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
			Expect(output.Security.M2M.ExternalIDP.TokenRequest).To(Equal("basic"))
			Expect(output.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
			Expect(output.Security.M2M.ExternalIDP.Client).ToNot(BeNil())
			Expect(output.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("client-id"))
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
