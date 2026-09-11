// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
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

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(HaveLen(5))
			Expect(secrets).To(Equal(map[string]string{
				"externalSecrets/api1/clientSecret":             "topsecret",
				"externalSecrets/api2/clientSecret":             "other-topsecret",
				"externalSecrets/api1/externalIDP/clientSecret": "secret123",
				"externalSecrets/api2/externalIDP/password":     "basicpassword",
				"externalSecrets/api3/basicAuth/password":       "basicpassword",
			}))
		})

		It("should return an empty map for no secrets", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: nil,
					Exposures:     nil,
				},
			}

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(BeEmpty())
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

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(BeEmpty())
		})

		It("should skip values that are already secret references", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientSecret: "$<existing:ref:checksum>",
										},
									},
								},
							},
						},
					},
				},
			}

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(HaveLen(1))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/clientSecret", "$<existing:ref:checksum>"))
		})

		It("should handle mixed refs and plain values", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientSecret: "$<existing:ref:checksum>",
											RefreshToken: "token",
										},
									},
								},
							},
						},
					},
				},
			}

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(HaveLen(2))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/clientSecret", "$<existing:ref:checksum>"))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/refreshToken", "token"))
		})

		It("should extract refreshToken secrets from subscriptions", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientSecret: "token",
											RefreshToken: "token",
										},
									},
								},
							},
						},
					},
				},
			}

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(HaveLen(2))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/clientSecret", "token"))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/refreshToken", "token"))
		})

		It("should extract refreshToken secrets from exposures", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Exposures: []roverv1.Exposure{
						{
							Api: &roverv1.ApiExposure{
								BasePath: "/api1",
								Security: &roverv1.Security{
									M2M: &roverv1.Machine2MachineAuthentication{
										ExternalIDP: &roverv1.ExternalIdentityProvider{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "token",
												RefreshToken: "token",
											},
										},
									},
								},
							},
						},
					},
				},
			}

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(HaveLen(2))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/externalIDP/clientSecret", "token"))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/externalIDP/refreshToken", "token"))
		})

		It("should handle subscription with nil Api", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{Api: nil},
					},
					Exposures: []roverv1.Exposure{
						{Api: nil},
					},
				},
			}

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(BeEmpty())
		})

		It("should extract basic auth password from subscriptions", func() {
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
											Password: "password",
										},
									},
								},
							},
						},
					},
				},
			}

			secrets := GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(HaveLen(1))
			Expect(secrets).To(HaveKeyWithValue("externalSecrets/api1/password", "password"))
		})
	})

	Context("SetExternalSecrets", func() {
		It("should replace secret values with refs for subscriptions", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientSecret: "token",
										},
									},
								},
							},
						},
					},
				},
			}

			availableSecrets := map[string]string{
				"externalSecrets/api1/clientSecret": "some:id:clientSecret:checksum",
			}

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<some:id:clientSecret:checksum>"))
		})

		It("should replace secret values with refs for exposures", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Exposures: []roverv1.Exposure{
						{
							Api: &roverv1.ApiExposure{
								BasePath: "/api1",
								Security: &roverv1.Security{
									M2M: &roverv1.Machine2MachineAuthentication{
										ExternalIDP: &roverv1.ExternalIdentityProvider{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "secret",
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
				"externalSecrets/api1/externalIDP/clientSecret": "some:id:extSecret:checksum",
			}

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("$<some:id:extSecret:checksum>"))
		})

		It("should replace multiple secrets across subscriptions and exposures", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientSecret: "secret",
											RefreshToken: "token",
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
										Basic: &roverv1.BasicAuthCredentials{
											Username: "user",
											Password: "password",
										},
									},
								},
							},
						},
					},
					Exposures: []roverv1.Exposure{
						{
							Api: &roverv1.ApiExposure{
								BasePath: "/api3",
								Security: &roverv1.Security{
									M2M: &roverv1.Machine2MachineAuthentication{
										ExternalIDP: &roverv1.ExternalIdentityProvider{
											Client: &roverv1.OAuth2ClientCredentials{
												ClientSecret: "secret",
												RefreshToken: "token",
											},
											Basic: &roverv1.BasicAuthCredentials{
												Password: "password",
											},
										},
										Basic: &roverv1.BasicAuthCredentials{
											Password: "password",
										},
									},
								},
							},
						},
					},
				},
			}

			availableSecrets := map[string]string{
				"externalSecrets/api1/clientSecret":             "id:api1:clientSecret:cs",
				"externalSecrets/api1/refreshToken":             "id:api1:refreshToken:cs",
				"externalSecrets/api2/password":                 "id:api2:password:cs",
				"externalSecrets/api3/externalIDP/clientSecret": "id:api3:extCS:cs",
				"externalSecrets/api3/externalIDP/refreshToken": "id:api3:extRT:cs",
				"externalSecrets/api3/externalIDP/password":     "id:api3:extPass:cs",
				"externalSecrets/api3/basicAuth/password":       "id:api3:basicPass:cs",
			}

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())

			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<id:api1:clientSecret:cs>"))
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.RefreshToken).To(Equal("$<id:api1:refreshToken:cs>"))
			Expect(rover.Spec.Subscriptions[1].Api.Security.M2M.Basic.Password).To(Equal("$<id:api2:password:cs>"))
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("$<id:api3:extCS:cs>"))
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Client.RefreshToken).To(Equal("$<id:api3:extRT:cs>"))
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Basic.Password).To(Equal("$<id:api3:extPass:cs>"))
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.Basic.Password).To(Equal("$<id:api3:basicPass:cs>"))
		})

		It("should not modify fields when secret is not in available secrets", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientSecret: "secret",
										},
									},
								},
							},
						},
					},
				},
			}

			availableSecrets := map[string]string{} // empty

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("secret"))
		})

		It("should handle empty rover spec", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: nil,
					Exposures:     nil,
				},
			}

			availableSecrets := map[string]string{
				"externalSecrets/api1/clientSecret": "some:ref",
			}

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())
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
											ClientSecret: "", // empty
										},
									},
								},
							},
						},
					},
				},
			}

			availableSecrets := map[string]string{
				"externalSecrets/api1/clientSecret": "some:id:cs",
			}

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal(""))
		})

		It("should only replace non-empty fields when multiple subscriptions exist", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientSecret: "token",
											RefreshToken: "", // empty — should not be replaced
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
											ClientSecret: "", // empty — should not be replaced
											RefreshToken: "token",
										},
									},
								},
							},
						},
					},
				},
			}

			availableSecrets := map[string]string{
				"externalSecrets/api1/clientSecret": "id:api1:cs",
				"externalSecrets/api1/refreshToken": "id:api1:rt",
				"externalSecrets/api2/clientSecret": "id:api2:cs",
				"externalSecrets/api2/refreshToken": "id:api2:rt",
			}

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())

			// api1: clientSecret replaced, refreshToken left empty
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<id:api1:cs>"))
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.RefreshToken).To(Equal(""))

			// api2: clientSecret left empty, refreshToken replaced
			Expect(rover.Spec.Subscriptions[1].Api.Security.M2M.Client.ClientSecret).To(Equal(""))
			Expect(rover.Spec.Subscriptions[1].Api.Security.M2M.Client.RefreshToken).To(Equal("$<id:api2:rt>"))
		})

		It("should preserve non-secret fields", func() {
			rover := &roverv1.Rover{
				Spec: roverv1.RoverSpec{
					Subscriptions: []roverv1.Subscription{
						{
							Api: &roverv1.ApiSubscription{
								BasePath: "/api1",
								Security: &roverv1.SubscriberSecurity{
									M2M: &roverv1.SubscriberMachine2MachineAuthentication{
										Client: &roverv1.OAuth2ClientCredentials{
											ClientId:     "my-client-id",
											ClientSecret: "token",
										},
									},
								},
							},
						},
					},
				},
			}

			availableSecrets := map[string]string{
				"externalSecrets/api1/clientSecret": "id:cs",
			}

			err := SetExternalSecrets(context.Background(), rover, availableSecrets)
			Expect(err).NotTo(HaveOccurred())
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientId).To(Equal("my-client-id"))
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<id:cs>"))
			Expect(rover.Spec.Subscriptions[0].Api.BasePath).To(Equal("/api1"))
		})
	})

	Context("OnboardApplication", func() {
		var testCtx context.Context
		var rover *roverv1.Rover

		BeforeEach(func() {
			testCtx = context.Background()
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
			fakeSecretManager.EXPECT().UpsertApplication(testCtx, "test", "eni--hyperion", "test-rover", mock.Anything).RunAndReturn(runAndReturnApplication)

			err := OnboardApplication(testCtx, rover, fakeSecretManager)
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
			fakeSecretManager.EXPECT().UpsertApplication(testCtx, "test", "eni--hyperion", "test-rover", onboardingOption, onboardingOption, onboardingOption).RunAndReturn(runAndReturnApplication)

			err := OnboardApplication(testCtx, rover, fakeSecretManager)
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
			fakeSecretManager.EXPECT().UpsertApplication(testCtx, "test", "eni--hyperion", "test-rover").RunAndReturn(runAndReturnApplication)

			err := OnboardApplication(testCtx, rover, fakeSecretManager)
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
			fakeSecretManager.EXPECT().UpsertApplication(testCtx, "test", "eni--hyperion", "test-rover").RunAndReturn(runAndReturnApplication)

			err := OnboardApplication(testCtx, rover, fakeSecretManager)
			Expect(err).NotTo(HaveOccurred())

			Expect(rover.Spec.ClientSecret).To(Equal("$<existing:clientSecret:checksum>"))
			Expect(rover.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientSecret).To(Equal("$<existing:clientSecret:checksum>"))
			Expect(rover.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Basic.Password).To(Equal("$<existing:externalIDPPassword:checksum>"))
		})
	})
})
