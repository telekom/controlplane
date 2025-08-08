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

var _ = Describe("Subscription Security Mapper (Out)", func() {
	Context("mapSubscriptionSecurity", func() {
		It("must map BasicAuth security correctly", func() {
			// Given
			input := &roverv1.ApiSubscription{
				BasePath: "/test",
				Security: &roverv1.SubscriberSecurity{
					M2M: &roverv1.SubscriberMachine2MachineAuthentication{
						Basic: &roverv1.BasicAuthCredentials{
							Username: "testuser",
							Password: "testpass",
						},
					},
				},
			}

			output := &api.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			basicAuth, err := output.Security.AsBasicAuth()
			Expect(err).To(BeNil())
			Expect(basicAuth.Username).To(Equal("testuser"))
			Expect(basicAuth.Password).To(Equal("testpass"))
			snaps.MatchSnapshot(GinkgoT(), basicAuth)
		})

		It("must map OAuth2 client credentials correctly", func() {
			// Given
			input := &roverv1.ApiSubscription{
				BasePath: "/test",
				Security: &roverv1.SubscriberSecurity{
					M2M: &roverv1.SubscriberMachine2MachineAuthentication{
						Client: &roverv1.OAuth2ClientCredentials{
							ClientId:     "client-id",
							ClientSecret: "client-secret",
							ClientKey:    "client-key",
						},
					},
				},
			}

			output := &api.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).To(BeNil())
			Expect(oauth2.ClientId).To(Equal("client-id"))
			Expect(oauth2.ClientSecret).To(Equal("client-secret"))
			Expect(oauth2.ClientKey).To(Equal("client-key"))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})

		It("must map OAuth2 scopes correctly", func() {
			// Given
			input := &roverv1.ApiSubscription{
				BasePath: "/test",
				Security: &roverv1.SubscriberSecurity{
					M2M: &roverv1.SubscriberMachine2MachineAuthentication{
						Scopes: []string{"read", "write"},
					},
				},
			}

			output := &api.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).To(BeNil())
			Expect(oauth2.Scopes).To(ContainElements("read", "write"))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})

		It("must handle nil security", func() {
			// Given
			input := &roverv1.ApiSubscription{
				BasePath: "/test",
				// Security is nil
			}

			output := &api.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).To(BeZero())
		})

		It("must handle nil M2M security", func() {
			// Given
			input := &roverv1.ApiSubscription{
				BasePath: "/test",
				Security: &roverv1.SubscriberSecurity{
					// M2M is nil
				},
			}

			output := &api.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).To(BeZero())
		})

		It("must map combined client credentials and scopes", func() {
			// Given
			input := &roverv1.ApiSubscription{
				BasePath: "/test",
				Security: &roverv1.SubscriberSecurity{
					M2M: &roverv1.SubscriberMachine2MachineAuthentication{
						Client: &roverv1.OAuth2ClientCredentials{
							ClientId:     "client-id",
							ClientSecret: "client-secret",
						},
						Scopes: []string{"read", "write"},
					},
				},
			}

			output := &api.ApiSubscription{}

			// When
			mapSubscriptionSecurity(input, output)

			// Then
			Expect(output.Security).ToNot(BeZero())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).To(BeNil())
			Expect(oauth2.ClientId).To(Equal("client-id"))
			Expect(oauth2.Scopes).To(ContainElements("read", "write"))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})
	})
})
