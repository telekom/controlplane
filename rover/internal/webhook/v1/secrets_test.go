// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"context"
	"errors"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	webhookv1 "github.com/telekom/controlplane/rover/internal/webhook/v1"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Secrets Handling", func() {
	var fakeSecretManager *fake.MockSecretManager

	BeforeEach(func() {
		fakeSecretManager = fake.NewMockSecretManager(GinkgoT())
	})

	Context("GetExternalSecrets", func() {
		It("should return an empty map for no secrets", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: nil,
					Exposures:     nil,
				},
			}

			secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
			Expect(err).To(Not(HaveOccurred()))
			Expect(secrets).To(BeEmpty())
		})

		Context("ApiExposure and ApiSubscription", func() {
			It("should return a map of external secrets", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "topsecret",
											},
										},
									},
								},
							},
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api2",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "other-topsecret",
											},
										},
									},
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											ExternalIDP: &roverv1.ExternalIdentityProvider{
												Client: &roverv1.OAuth2ClientCredentials{
													ClientSecret: "secret123",
												},
											},
										},
									},
								},
							},
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api2",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											ExternalIDP: &roverv1.ExternalIdentityProvider{
												Basic: &roverv1.BasicAuthCredentials{
													Password: "basicpassword",
												},
											},
										},
									},
								},
							},
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api3",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Password: "basicpassword",
											},
										},
									},
								},
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(HaveLen(5))
				Expect(secrets).To(Equal(map[string]string{
					"externalSecrets/api1/clientSecret":             "topsecret",
					"externalSecrets/api2/clientSecret":             "other-topsecret",
					"externalSecrets/api1/externalIDP/clientSecret": "secret123",
					"externalSecrets/api2/externalIDP/password":     "basicpassword",
					"externalSecrets/api3/basicAuth/password":       "basicpassword",
				}))
			})

			It("should extract basic auth passwords from subscriptions", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Username: "user",
												Password: "subscription-basic-password",
											},
										},
									},
								},
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(Equal(map[string]string{
					"externalSecrets/api1/password": "subscription-basic-password",
				}))
			})

			It("should collect all non-empty secret values, including references", func() {
				// GetExternalSecrets does not filter out reference values; that
				// filtering happens later in OnboardApplication.
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "$<existing:clientSecret:checksum>",
											},
										},
									},
								},
							},
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api2",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "plain-secret",
											},
										},
									},
								},
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(Equal(map[string]string{
					"externalSecrets/api1/clientSecret": "$<existing:clientSecret:checksum>",
					"externalSecrets/api2/clientSecret": "plain-secret",
				}))
			})

			It("should handle nil security fields gracefully", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: nil, // No security defined
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api2",
									Security: nil, // No security defined
								},
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(BeEmpty())
			})

			It("should handle nil api fields gracefully", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: nil, // No API defined
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Api: nil, // No API defined
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(BeEmpty())
			})

			It("should handle security without m2m gracefully", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: nil, // Security set but no M2M
									},
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api2",
									Security: &roverv1.Security{
										M2M: nil, // Security set but no M2M
									},
								},
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(BeEmpty())
			})
		})

		Context("AiExposure and AiSubscription", func() {
			It("should collect secrets from AI exposures and subscriptions like API", func() {
				// AI/MCP secrets must be collected exactly like their .Api counterparts.
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Ai: &roverv1.AiSubscription{
									BasePath: "/ai1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "ai-consumer-secret",
											},
										},
									},
								},
							},
							{
								Ai: &roverv1.AiSubscription{
									BasePath: "/ai2",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Username: "user",
												Password: "ai-consumer-password",
											},
										},
									},
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Ai: &roverv1.AiExposure{
									BasePath: "/ai1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											ExternalIDP: &roverv1.ExternalIdentityProvider{
												Client: &roverv1.OAuth2ClientCredentials{
													ClientSecret: "ai-externalidp-secret",
												},
											},
										},
									},
								},
							},
							{
								Ai: &roverv1.AiExposure{
									BasePath: "/ai2",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Password: "ai-basic-password",
											},
										},
									},
								},
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(Equal(map[string]string{
					"externalSecrets/ai1/clientSecret":             "ai-consumer-secret",
					"externalSecrets/ai2/password":                 "ai-consumer-password",
					"externalSecrets/ai1/externalIDP/clientSecret": "ai-externalidp-secret",
					"externalSecrets/ai2/basicAuth/password":       "ai-basic-password",
				}))
			})
		})

		Context("Api and Ai (Exposure and Subscription)", func() {
			It("should collect secrets from both API and AI resources", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "api-consumer-secret",
											},
										},
									},
								},
							},
							{
								Ai: &roverv1.AiSubscription{
									BasePath: "/ai1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "ai-consumer-secret",
											},
										},
									},
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Password: "api-basic-password",
											},
										},
									},
								},
							},
							{
								Ai: &roverv1.AiExposure{
									BasePath: "/ai1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											ExternalIDP: &roverv1.ExternalIdentityProvider{
												Client: &roverv1.OAuth2ClientCredentials{
													ClientSecret: "ai-externalidp-secret",
												},
											},
										},
									},
								},
							},
						},
					},
				}

				secrets, err := webhookv1.GetExternalSecrets(context.Background(), rover)
				Expect(err).To(Not(HaveOccurred()))
				Expect(secrets).To(Equal(map[string]string{
					"externalSecrets/api1/clientSecret":            "api-consumer-secret",
					"externalSecrets/api1/basicAuth/password":      "api-basic-password",
					"externalSecrets/ai1/clientSecret":             "ai-consumer-secret",
					"externalSecrets/ai1/externalIDP/clientSecret": "ai-externalidp-secret",
				}))
			})
		})
	})

	Context("SetExternalSecrets", func() {
		Context("ApiExposure and ApiSubscription", func() {
			It("should replace non-empty secret values with their references", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "topsecret",
											},
										},
									},
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api2",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Password: "basicpassword",
											},
										},
									},
								},
							},
						},
					},
				}

				availableSecrets := map[string]string{
					"externalSecrets/api1/clientSecret":       "id:externalSecrets/api1/clientSecret:checksum",
					"externalSecrets/api2/basicAuth/password": "id:externalSecrets/api2/basicAuth/password:checksum",
				}

				err := webhookv1.SetExternalSecrets(context.Background(), rover, availableSecrets)
				Expect(err).NotTo(HaveOccurred())

				Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<id:externalSecrets/api1/clientSecret:checksum>"))
				Expect(rover.Spec.Exposures[0].Api.Security.M2M.Basic.Password).To(Equal("$<id:externalSecrets/api2/basicAuth/password:checksum>"))
			})

			It("should replace subscription basic auth passwords with their references", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Username: "user",
												Password: "subscription-basic-password",
											},
										},
									},
								},
							},
						},
					},
				}

				availableSecrets := map[string]string{
					"externalSecrets/api1/password": "id:externalSecrets/api1/password:checksum",
				}

				err := webhookv1.SetExternalSecrets(context.Background(), rover, availableSecrets)
				Expect(err).NotTo(HaveOccurred())

				Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Basic.Password).To(Equal("$<id:externalSecrets/api1/password:checksum>"))
			})

			It("should replace exposure externalIDP client secrets with their references", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Exposures: []roverv1.Exposure{
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											ExternalIDP: &roverv1.ExternalIdentityProvider{
												TokenEndpoint: "https://example.com/token",
												Client: &roverv1.OAuth2ClientCredentials{
													ClientId:     "client-id",
													ClientSecret: "externalidp-client-secret",
												},
											},
										},
									},
								},
							},
						},
					},
				}

				availableSecrets := map[string]string{
					"externalSecrets/api1/externalIDP/clientSecret": "id:externalSecrets/api1/externalIDP/clientSecret:checksum",
				}

				err := webhookv1.SetExternalSecrets(context.Background(), rover, availableSecrets)
				Expect(err).NotTo(HaveOccurred())

				Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("$<id:externalSecrets/api1/externalIDP/clientSecret:checksum>"))
			})

			It("should leave values unchanged when no matching secret is available", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "topsecret",
											},
										},
									},
								},
							},
						},
					},
				}

				err := webhookv1.SetExternalSecrets(context.Background(), rover, map[string]string{})
				Expect(err).NotTo(HaveOccurred())

				Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("topsecret"))
			})

			It("should skip empty secret values", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "",
											},
										},
									},
								},
							},
						},
					},
				}

				err := webhookv1.SetExternalSecrets(context.Background(), rover, map[string]string{
					"externalSecrets/api1/clientSecret": "id:externalSecrets/api1/clientSecret:checksum",
				})
				Expect(err).NotTo(HaveOccurred())

				Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(BeEmpty())
			})
		})

		Context("AiExposure and AiSubscription", func() {
			It("should replace AI secret values with their references like API", func() {
				// AI/MCP secret values must be replaced with references exactly like
				// their .Api counterparts.
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Ai: &roverv1.AiSubscription{
									BasePath: "/ai1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "ai-consumer-secret",
											},
										},
									},
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Ai: &roverv1.AiExposure{
									BasePath: "/ai1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Password: "ai-basic-password",
											},
										},
									},
								},
							},
						},
					},
				}

				availableSecrets := map[string]string{
					"externalSecrets/ai1/clientSecret":       "id:externalSecrets/ai1/clientSecret:checksum",
					"externalSecrets/ai1/basicAuth/password": "id:externalSecrets/ai1/basicAuth/password:checksum",
				}

				err := webhookv1.SetExternalSecrets(context.Background(), rover, availableSecrets)
				Expect(err).NotTo(HaveOccurred())

				Expect(rover.Spec.Subscriptions[0].Ai.Security.M2M.Client.ClientSecret).To(Equal("$<id:externalSecrets/ai1/clientSecret:checksum>"))
				Expect(rover.Spec.Exposures[0].Ai.Security.M2M.Basic.Password).To(Equal("$<id:externalSecrets/ai1/basicAuth/password:checksum>"))
			})
		})

		Context("Api and Ai (Exposure and Subscription)", func() {
			It("should replace secrets on both API and AI resources", func() {
				rover := &roverv1.Rover{
					Spec: roverv1.RoverSpec{
						Subscriptions: []roverv1.Subscription{
							{
								Api: &roverv1.ApiSubscription{
									BasePath: "/api1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "api-consumer-secret",
											},
										},
									},
								},
							},
							{
								Ai: &roverv1.AiSubscription{
									BasePath: "/ai1",
									Security: &roverv1.SubscriberSecurity{
										M2M: &roverv1.SubscriberMachine2MachineAuthentication{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "ai-consumer-secret",
											},
										},
									},
								},
							},
						},
						Exposures: []roverv1.Exposure{
							{
								Api: &roverv1.ApiExposure{
									BasePath: "/api1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Password: "api-basic-password",
											},
										},
									},
								},
							},
							{
								Ai: &roverv1.AiExposure{
									BasePath: "/ai1",
									Security: &roverv1.Security{
										M2M: &roverv1.Machine2MachineAuthentication{
											Basic: &roverv1.BasicAuthCredentials{
												Password: "ai-basic-password",
											},
										},
									},
								},
							},
						},
					},
				}

				availableSecrets := map[string]string{
					"externalSecrets/api1/clientSecret":       "id:externalSecrets/api1/clientSecret:checksum",
					"externalSecrets/api1/basicAuth/password": "id:externalSecrets/api1/basicAuth/password:checksum",
					"externalSecrets/ai1/clientSecret":        "id:externalSecrets/ai1/clientSecret:checksum",
					"externalSecrets/ai1/basicAuth/password":  "id:externalSecrets/ai1/basicAuth/password:checksum",
				}

				err := webhookv1.SetExternalSecrets(context.Background(), rover, availableSecrets)
				Expect(err).NotTo(HaveOccurred())

				Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<id:externalSecrets/api1/clientSecret:checksum>"))
				Expect(rover.Spec.Subscriptions[1].Ai.Security.M2M.Client.ClientSecret).To(Equal("$<id:externalSecrets/ai1/clientSecret:checksum>"))
				Expect(rover.Spec.Exposures[0].Api.Security.M2M.Basic.Password).To(Equal("$<id:externalSecrets/api1/basicAuth/password:checksum>"))
				Expect(rover.Spec.Exposures[1].Ai.Security.M2M.Basic.Password).To(Equal("$<id:externalSecrets/ai1/basicAuth/password:checksum>"))
			})
		})
	})

	Context("OnboardApplication", func() {
		var ctx context.Context
		var rover *roverv1.Rover

		BeforeEach(func() {
			ctx = context.Background()
			rover = &roverv1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rover",
					Namespace: "default--eni--hyperion",
					Labels: map[string]string{
						config.EnvironmentLabelKey: "test",
					},
				},
				Spec: roverv1.RoverSpec{},
			}
		})

		It("should onboard an application with no external secrets", func() {
			runAndReturnApplication := func(ctx context.Context, envId, teamId, appId string, opts ...api.OnboardingOption) (map[string]string, error) {
				Expect(opts).To(BeEmpty())

				return map[string]string{
					"clientSecret": "some:id:clientSecret:checksum",
				}, nil
			}
			fakeSecretManager.EXPECT().UpsertApplication(ctx, "test", "eni--hyperion", "test-rover", mock.Anything).RunAndReturn(runAndReturnApplication)

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should onboard an application with external secrets", func() {
			rover.Spec.ClientSecret = "topsecret-value"
			rover.Spec.Subscriptions = []roverv1.Subscription{
				{
					Api: &roverv1.ApiSubscription{
						BasePath: "/api1",
						Security: &roverv1.SubscriberSecurity{
							M2M: &roverv1.SubscriberMachine2MachineAuthentication{
								Client: &roverv1.OAuth2ClientCredentials{
									ClientId:     "client-id",
									ClientSecret: "client-secret-value",
								},
							},
						},
					},
				},
			}
			rover.Spec.Exposures = []roverv1.Exposure{
				{
					Api: &roverv1.ApiExposure{
						BasePath: "/api1",
						Security: &roverv1.Security{
							M2M: &roverv1.Machine2MachineAuthentication{
								ExternalIDP: &roverv1.ExternalIdentityProvider{
									TokenEndpoint: "https://example.com/token",
									Basic: &roverv1.BasicAuthCredentials{
										Username: "user",
										Password: "password-value",
									},
								},
							},
						},
					},
				},
			}

			runAndReturnApplication := func(ctx context.Context, envId, teamId, appId string, opts ...api.OnboardingOption) (map[string]string, error) {
				Expect(opts).To(HaveLen(3))
				options := &api.OnboardingOptions{}
				for _, opt := range opts {
					opt(options)
				}
				Expect(options.SecretValues).To(HaveKeyWithValue("clientSecret", "topsecret-value"))
				Expect(options.SecretValues).To(HaveKeyWithValue("externalSecrets/api1/clientSecret", "client-secret-value"))
				Expect(options.SecretValues).To(HaveKeyWithValue("externalSecrets/api1/externalIDP/password", "password-value"))

				return map[string]string{
					"clientSecret":                              "some:id:clientSecret:checksum",
					"externalSecrets/api1/clientSecret":         "some:id:externalSecrets/api1/clientSecret:checksum",
					"externalSecrets/api1/externalIDP/password": "some:id:externalSecrets/api1/externalIDP/password:checksum",
				}, nil
			}

			onboardingOption := mock.AnythingOfType("api.OnboardingOption")
			fakeSecretManager.EXPECT().UpsertApplication(ctx, "test", "eni--hyperion", "test-rover", onboardingOption, onboardingOption, onboardingOption).RunAndReturn(runAndReturnApplication)

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).NotTo(HaveOccurred())

			Expect(rover.Spec.ClientSecret).To(Equal("$<some:id:clientSecret:checksum>"))
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<some:id:externalSecrets/api1/clientSecret:checksum>"))
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Basic.Password).To(Equal("$<some:id:externalSecrets/api1/externalIDP/password:checksum>"))
		})

		It("should only update the clientSecret if it is not a reference", func() {
			rover.Spec.ClientSecret = "$<existing:clientSecret:checksum>"

			runAndReturnApplication := func(ctx context.Context, envId, teamId, appId string, opts ...api.OnboardingOption) (map[string]string, error) {
				Expect(opts).To(BeEmpty()) // the important check is that the secret is not set as value here

				return map[string]string{
					"clientSecret": "existing:clientSecret:checksum", // The SM will return the current value (which should match the existing reference)
				}, nil
			}
			fakeSecretManager.EXPECT().UpsertApplication(ctx, "test", "eni--hyperion", "test-rover").RunAndReturn(runAndReturnApplication)

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).NotTo(HaveOccurred())

			Expect(rover.Spec.ClientSecret).To(Equal("$<existing:clientSecret:checksum>"))
		})

		It("should skip secrets that already are a reference", func() {
			rover.Spec.ClientSecret = "$<existing:clientSecret:checksum>"
			rover.Spec.Subscriptions = []roverv1.Subscription{
				{
					Api: &roverv1.ApiSubscription{
						BasePath: "/api1",
						Security: &roverv1.SubscriberSecurity{
							M2M: &roverv1.SubscriberMachine2MachineAuthentication{
								Client: &roverv1.OAuth2ClientCredentials{
									ClientSecret: "$<existing:clientSecret:checksum>",
								},
							},
						},
					},
				},
			}
			rover.Spec.Exposures = []roverv1.Exposure{
				{
					Api: &roverv1.ApiExposure{
						BasePath: "/api1",
						Security: &roverv1.Security{
							M2M: &roverv1.Machine2MachineAuthentication{
								ExternalIDP: &roverv1.ExternalIdentityProvider{
									TokenEndpoint: "https://example.com/token",
									Basic: &roverv1.BasicAuthCredentials{
										Username: "user",
										Password: "$<existing:externalIDPPassword:checksum>",
									},
								},
							},
						},
					},
				},
			}

			runAndReturnApplication := func(ctx context.Context, envId, teamId, appId string, opts ...api.OnboardingOption) (map[string]string, error) {
				Expect(opts).To(BeEmpty()) // No new secrets should be set

				return map[string]string{
					"clientSecret":    "existing:clientSecret:checksum",
					"externalSecrets": `{"api1": {"clientSecret": "existing:clientSecret:checksum", "externalIDP": {"password": "existing:externalIDPPassword:checksum"}}}`,
				}, nil
			}
			fakeSecretManager.EXPECT().UpsertApplication(ctx, "test", "eni--hyperion", "test-rover").RunAndReturn(runAndReturnApplication)

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).NotTo(HaveOccurred())

			Expect(rover.Spec.ClientSecret).To(Equal("$<existing:clientSecret:checksum>"))
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<existing:clientSecret:checksum>"))
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Basic.Password).To(Equal("$<existing:externalIDPPassword:checksum>"))
		})

		It("should skip onboarding when the Secret Manager feature is disabled", func() {
			config.SetFeatureEnabled(config.FeatureSecretManager, false)
			defer config.SetFeatureEnabled(config.FeatureSecretManager, true)

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).NotTo(HaveOccurred())
			// fakeSecretManager has no expectations set, so any call would fail the test
		})

		It("should be a no-op when the secret manager is nil", func() {
			err := webhookv1.OnboardApplication(ctx, rover, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when the environment label is missing", func() {
			rover.Labels = nil

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("environment label is required"))
		})

		It("should return an error when UpsertApplication fails", func() {
			fakeSecretManager.EXPECT().
				UpsertApplication(ctx, "test", "eni--hyperion", "test-rover", mock.Anything).
				Return(nil, errors.New("boom"))

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to onboard application"))
		})

		It("should return an error when the clientSecret is not returned by the secret manager", func() {
			fakeSecretManager.EXPECT().
				UpsertApplication(ctx, "test", "eni--hyperion", "test-rover", mock.Anything).
				Return(map[string]string{}, nil)

			err := webhookv1.OnboardApplication(ctx, rover, fakeSecretManager)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("clientSecret not found in available secrets"))
		})
	})
})
