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
	"github.com/telekom/controlplane/rover-server/internal/mapper/rover/in"
)

var _ = Describe("Subscription Security Mapper (Out)", func() {
	Context("mapSubscriptionSecurity", func() {
		It("must map the subscriber grant type", func() {
			input := &roverv1.ApiSubscription{
				BasePath: "/test",
				Security: &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
					Client:    &roverv1.OAuth2ClientCredentials{ClientId: "client-id", ClientSecret: "client-secret", RefreshToken: "rt"},
					GrantType: roverv1.GrantTypeRefreshToken,
				}},
			}
			output := &api.ApiSubscription{}

			Expect(mapSubscriptionSecurity(input, output)).To(Succeed())

			oauth2, err := output.Security.AsOauth2()
			Expect(err).NotTo(HaveOccurred())
			Expect(oauth2.GrantType).To(Equal(api.RefreshToken))
			snaps.MatchSnapshot(GinkgoT(), oauth2)
		})

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
							RefreshToken: "refreshToken",
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
			Expect(oauth2.RefreshToken).To(Equal("refreshToken"))
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

		It("must map combined password-grant credentials correctly", func() {
			input := &roverv1.ApiSubscription{
				Security: &roverv1.SubscriberSecurity{
					M2M: &roverv1.SubscriberMachine2MachineAuthentication{
						Client: &roverv1.OAuth2ClientCredentials{
							ClientId:     "client-id",
							ClientSecret: "client-secret",
						},
						Basic: &roverv1.BasicAuthCredentials{
							Username: "testuser",
							Password: "testpass",
						},
						Scopes: []string{"read", "write"},
					},
				},
			}

			output := &api.ApiSubscription{}
			err := mapSubscriptionSecurity(input, output)

			Expect(err).ToNot(HaveOccurred())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).ToNot(HaveOccurred())
			Expect(oauth2.ClientId).To(Equal("client-id"))
			Expect(oauth2.ClientSecret).To(Equal("client-secret"))
			Expect(oauth2.Username).To(Equal("testuser"))
			Expect(oauth2.Password).To(Equal("testpass"))
			Expect(oauth2.Scopes).To(ContainElements("read", "write"))
		})

		It("must keep username/password with a grant type as oauth2", func() {
			input := &roverv1.ApiSubscription{
				Security: &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
					Basic:     &roverv1.BasicAuthCredentials{Username: "testuser", Password: "testpass"},
					GrantType: roverv1.GrantTypePassword,
				}},
			}
			output := &api.ApiSubscription{}

			Expect(mapSubscriptionSecurity(input, output)).To(Succeed())

			discriminator, err := output.Security.Discriminator()
			Expect(err).ToNot(HaveOccurred())
			Expect(discriminator).To(Equal("oauth2"))
			oauth2, err := output.Security.AsOauth2()
			Expect(err).ToNot(HaveOccurred())
			Expect(oauth2.Username).To(Equal("testuser"))
			Expect(oauth2.Password).To(Equal("testpass"))
			Expect(oauth2.GrantType).To(Equal(api.Password))
		})
	})

	DescribeTable("maps the subscriber token request method",
		func(tokenRequest roverv1.TokenRequestMethod, expected api.Oauth2TokenRequest) {
			security := &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
				Client:       &roverv1.OAuth2ClientCredentials{ClientId: "client-id", ClientSecret: "client-secret"},
				TokenRequest: tokenRequest,
			}}

			apiOut := &api.ApiSubscription{}
			Expect(mapSubscriptionSecurity(&roverv1.ApiSubscription{Security: security}, apiOut)).To(Succeed())
			apiOauth2, err := apiOut.Security.AsOauth2()
			Expect(err).NotTo(HaveOccurred())
			Expect(apiOauth2.TokenRequest).To(Equal(expected))

			aiOut, err := mapAiSubscription(&roverv1.AgenticSubscription{Security: security})
			Expect(err).NotTo(HaveOccurred())
			aiOauth2, err := aiOut.Security.AsOauth2()
			Expect(err).NotTo(HaveOccurred())
			Expect(aiOauth2.TokenRequest).To(Equal(expected))
		},
		Entry("omitted", roverv1.TokenRequestMethod(""), api.Oauth2TokenRequest("")),
		Entry("client_secret_basic", roverv1.TokenRequestClientSecretBasic, api.Oauth2TokenRequestHeader),
		Entry("client_secret_post", roverv1.TokenRequestClientSecretPost, api.Oauth2TokenRequestBody),
	)

	Context("mapAiSubscription", func() {
		It("must keep username/password with a grant type as oauth2", func() {
			input := &roverv1.AgenticSubscription{
				Security: &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
					Basic:     &roverv1.BasicAuthCredentials{Username: "testuser", Password: "testpass"},
					GrantType: roverv1.GrantTypePassword,
				}},
			}

			output, err := mapAiSubscription(input)

			Expect(err).ToNot(HaveOccurred())
			discriminator, err := output.Security.Discriminator()
			Expect(err).ToNot(HaveOccurred())
			Expect(discriminator).To(Equal("oauth2"))
			oauth2, err := output.Security.AsOauth2()
			Expect(err).ToNot(HaveOccurred())
			Expect(oauth2.Username).To(Equal("testuser"))
			Expect(oauth2.Password).To(Equal("testpass"))
			Expect(oauth2.GrantType).To(Equal(api.Password))
		})

		It("must map combined password-grant credentials correctly", func() {
			input := &roverv1.AgenticSubscription{
				Security: &roverv1.SubscriberSecurity{
					M2M: &roverv1.SubscriberMachine2MachineAuthentication{
						Client: &roverv1.OAuth2ClientCredentials{
							ClientId:     "client-id",
							ClientSecret: "client-secret",
						},
						Basic: &roverv1.BasicAuthCredentials{
							Username: "testuser",
							Password: "testpass",
						},
						Scopes: []string{"read", "write"},
					},
				},
			}

			output, err := mapAiSubscription(input)

			Expect(err).ToNot(HaveOccurred())
			oauth2, err := output.Security.AsOauth2()
			Expect(err).ToNot(HaveOccurred())
			Expect(oauth2.ClientId).To(Equal("client-id"))
			Expect(oauth2.ClientSecret).To(Equal("client-secret"))
			Expect(oauth2.Username).To(Equal("testuser"))
			Expect(oauth2.Password).To(Equal("testpass"))
			Expect(oauth2.Scopes).To(ContainElements("read", "write"))
		})
	})

	DescribeTable("round-trips subscriber security through the rover-server API",
		func(security *roverv1.SubscriberSecurity) {
			original := &roverv1.Rover{Spec: roverv1.RoverSpec{Subscriptions: []roverv1.Subscription{
				{Api: &roverv1.ApiSubscription{BasePath: "/api", Security: security.DeepCopy()}},
				{Agentic: &roverv1.AgenticSubscription{BasePath: "/ai", Security: security.DeepCopy()}},
			}}}

			response := &api.Rover{}
			Expect(MapRover(original, response)).To(Succeed())
			reapplied := &roverv1.Rover{}
			Expect(in.MapRover(response, reapplied)).To(Succeed())

			Expect(reapplied.Spec.Subscriptions).To(HaveLen(2))
			Expect(reapplied.Spec.Subscriptions[0].Api.Security).To(Equal(security))
			Expect(reapplied.Spec.Subscriptions[1].Agentic.Security).To(Equal(security))
		},
		Entry("backend Basic auth", &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
			Basic: &roverv1.BasicAuthCredentials{Username: "testuser", Password: "testpass"},
		}}),
		Entry("username/password with a password grant", &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
			Basic:     &roverv1.BasicAuthCredentials{Username: "testuser", Password: "testpass"},
			GrantType: roverv1.GrantTypePassword,
		}}),
		Entry("client and username/password with scopes and a password grant", &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
			Client:    &roverv1.OAuth2ClientCredentials{ClientId: "client-id", ClientSecret: "client-secret"},
			Basic:     &roverv1.BasicAuthCredentials{Username: "testuser", Password: "testpass"},
			Scopes:    []string{"read", "write"},
			GrantType: roverv1.GrantTypePassword,
		}}),
		Entry("client credentials with a client_secret_post token request", &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
			Client:       &roverv1.OAuth2ClientCredentials{ClientId: "client-id", ClientSecret: "client-secret"},
			Scopes:       []string{"read"},
			TokenRequest: roverv1.TokenRequestClientSecretPost,
		}}),
		Entry("client and username/password with a client_secret_basic token request", &roverv1.SubscriberSecurity{M2M: &roverv1.SubscriberMachine2MachineAuthentication{
			Client:       &roverv1.OAuth2ClientCredentials{ClientId: "client-id", ClientSecret: "client-secret"},
			Basic:        &roverv1.BasicAuthCredentials{Username: "testuser", Password: "testpass"},
			GrantType:    roverv1.GrantTypePassword,
			TokenRequest: roverv1.TokenRequestClientSecretBasic,
		}}),
	)
})
