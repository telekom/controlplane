// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	webhookv1 "github.com/telekom/controlplane/rover/internal/webhook/v1"
	"github.com/telekom/controlplane/secret-manager/api"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

			secrets := webhookv1.GetExternalSecrets(context.Background(), rover)
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

			secrets := webhookv1.GetExternalSecrets(context.Background(), rover)
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

			secrets := webhookv1.GetExternalSecrets(context.Background(), rover)
			Expect(secrets).To(BeEmpty())
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
				Expect(opts).To(HaveLen(0))

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
				Expect(opts).To(HaveLen(0)) // the important check is that the secret is not set as value here

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
				Expect(opts).To(HaveLen(0)) // No new secrets should be set

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

	})
})
