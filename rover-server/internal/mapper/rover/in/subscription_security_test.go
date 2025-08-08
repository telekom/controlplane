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

var _ = Describe("Subscription Security Mapper", func() {
	Context("mapSubscriptionSecurity", func() {
		It("must map BasicAuth security correctly", func() {
			// Given
			input := api.ApiSubscription{
				BasePath: "/test",
			}
			basicAuth := api.BasicAuth{
				Username: "testuser",
				Password: "testpass",
			}
			input.Security = api.Security{}
			err := input.Security.FromBasicAuth(basicAuth)
			Expect(err).To(BeNil())

			output := &roverv1.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Basic).ToNot(BeNil())
			Expect(output.Security.M2M.Basic.Username).To(Equal("testuser"))
			Expect(output.Security.M2M.Basic.Password).To(Equal("testpass"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must map OAuth2 client credentials correctly", func() {
			// Given
			input := api.ApiSubscription{
				BasePath: "/test",
			}
			oauth2 := api.Oauth2{
				ClientId:     "client-id",
				ClientSecret: "client-secret",
				ClientKey:    "client-key",
			}
			input.Security = api.Security{}
			err := input.Security.FromOauth2(oauth2)
			Expect(err).To(BeNil())

			output := &roverv1.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Client).ToNot(BeNil())
			Expect(output.Security.M2M.Client.ClientId).To(Equal("client-id"))
			Expect(output.Security.M2M.Client.ClientSecret).To(Equal("client-secret"))
			Expect(output.Security.M2M.Client.ClientKey).To(Equal("client-key"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must map OAuth2 username/password correctly", func() {
			// Given
			input := api.ApiSubscription{
				BasePath: "/test",
			}
			oauth2 := api.Oauth2{
				Username: "testuser",
				Password: "testpass",
			}
			input.Security = api.Security{}
			err := input.Security.FromOauth2(oauth2)
			Expect(err).To(BeNil())

			output := &roverv1.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Basic).ToNot(BeNil())
			Expect(output.Security.M2M.Basic.Username).To(Equal("testuser"))
			Expect(output.Security.M2M.Basic.Password).To(Equal("testpass"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must map OAuth2 scopes correctly", func() {
			// Given
			input := api.ApiSubscription{
				BasePath: "/test",
			}
			oauth2 := api.Oauth2{
				Scopes: []string{"read", "write"},
			}
			input.Security = api.Security{}
			err := input.Security.FromOauth2(oauth2)
			Expect(err).To(BeNil())

			output := &roverv1.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Scopes).To(ContainElements("read", "write"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})

		It("must handle invalid security discriminator", func() {
			// Given
			input := api.ApiSubscription{
				BasePath: "/test",
			}
			input.Security = api.Security{} // Invalid security without proper initialization

			output := &roverv1.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).To(BeNil()) // Should not set security when there's an error
		})

		It("must handle nil security", func() {
			// Given
			input := api.ApiSubscription{
				BasePath: "/test",
			}
			// Security is not set

			output := &roverv1.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).To(BeNil()) // Should not set security when it's nil
		})

		It("must map combined OAuth2 configuration correctly", func() {
			// Given
			input := api.ApiSubscription{
				BasePath: "/test",
			}
			oauth2 := api.Oauth2{
				ClientId:     "client-id",
				ClientSecret: "client-secret",
				Scopes:       []string{"read", "write"},
			}
			input.Security = api.Security{}
			err := input.Security.FromOauth2(oauth2)
			Expect(err).To(BeNil())

			output := &roverv1.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeNil())
			Expect(output.Security.M2M).ToNot(BeNil())
			Expect(output.Security.M2M.Client).ToNot(BeNil())
			Expect(output.Security.M2M.Client.ClientId).To(Equal("client-id"))
			Expect(output.Security.M2M.Scopes).To(ContainElements("read", "write"))
			snaps.MatchSnapshot(GinkgoT(), output.Security)
		})
	})
})
