// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featuresmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
)

var _ = Describe("BasicAuthFeature", func() {

	It("should return the correct feature type", func() {
		Expect(feature.InstanceBasicAuthFeature.Name()).To(Equal(gatewayv1.FeatureTypeBasicAuth))
	})

	It("should have the correct priority", func() {
		Expect(feature.InstanceBasicAuthFeature.Priority()).To(Equal(10))
	})

	Context("with mocked feature builder", func() {
		var ctrl *gomock.Controller
		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder

		BeforeEach(func() {

			ctrl = gomock.NewController(GinkgoT())
			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(ctrl)
		})

		Context("IsUsed", func() {
			It("should not be used when route has PassThrough=true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: true,
						Security: &gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "testuser",
									Password: "testpass",
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				Expect(feature.InstanceBasicAuthFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should not be used when route is a proxy and not failover", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Upstreams: []gatewayv1.Upstream{
							{
								IssuerUrl: "https://issuer.example.com", // Having an issuer makes it a proxy route
							},
						},
						Security: &gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "testuser",
									Password: "testpass",
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				Expect(feature.InstanceBasicAuthFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should be used when route has basic auth in primary security", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Security: &gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "testuser",
									Password: "testpass",
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				Expect(feature.InstanceBasicAuthFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeTrue())
			})

			It("should be used when route has basic auth in failover security", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Traffic: gatewayv1.Traffic{
							Failover: &gatewayv1.Failover{
								Security: &gatewayv1.Security{
									M2M: &gatewayv1.Machine2MachineAuthentication{
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "testuser",
											Password: "testpass",
										},
									},
								},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				Expect(feature.InstanceBasicAuthFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeTrue())
			})

			It("should not be used when no basic auth is configured", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Security: &gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								// No basic auth configured, only scopes
								Scopes: []string{"scope1", "scope2"},
							},
						},
					},
				}
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return(nil).Times(1)
				Expect(feature.InstanceBasicAuthFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("should be used when any allowed consumer has basic auth", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
					},
				}

				consumer := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						Security: &gatewayv1.ConsumeRouteSecurity{
							M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "consumeruser",
									Password: "consumerpass",
								},
							},
						},
					},
				}

				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).Times(1)
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer}).Times(1)

				Expect(feature.InstanceBasicAuthFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeTrue())
			})
		})

		Context("Apply", func() {
			It("should apply basic auth credentials from primary route security", func() {
				// Setup
				jumperConfig := plugin.NewJumperConfig()
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Security: &gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "testuser",
									Password: "testpass",
								},
							},
						},
					},
				}

				mockFeatureBuilder.EXPECT().JumperConfig().Return(jumperConfig)
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

				// Execute
				err := feature.InstanceBasicAuthFeature.Apply(context.Background(), mockFeatureBuilder)

				// Verify
				Expect(err).ToNot(HaveOccurred())
				defaultCreds := jumperConfig.BasicAuth[plugin.ConsumerId(feature.DefaultProviderKey)]
				Expect(defaultCreds.Username).To(Equal("testuser"))
				Expect(defaultCreds.Password).To(Equal("testpass"))
			})

			It("should apply basic auth credentials from failover security", func() {
				// Setup
				jumperConfig := plugin.NewJumperConfig()
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Traffic: gatewayv1.Traffic{
							Failover: &gatewayv1.Failover{
								Security: &gatewayv1.Security{
									M2M: &gatewayv1.Machine2MachineAuthentication{
										Basic: &gatewayv1.BasicAuthCredentials{
											Username: "failoveruser",
											Password: "failoverpass",
										},
									},
								},
							},
						},
					},
				}

				mockFeatureBuilder.EXPECT().JumperConfig().Return(jumperConfig)
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

				// Execute
				err := feature.InstanceBasicAuthFeature.Apply(context.Background(), mockFeatureBuilder)

				// Verify
				Expect(err).ToNot(HaveOccurred())
				defaultCreds := jumperConfig.BasicAuth[plugin.ConsumerId(feature.DefaultProviderKey)]
				Expect(defaultCreds.Username).To(Equal("failoveruser"))
				Expect(defaultCreds.Password).To(Equal("failoverpass"))
			})

			It("should apply basic auth credentials for allowed consumers", func() {
				// Setup
				jumperConfig := plugin.NewJumperConfig()
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Security: &gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "routeuser",
									Password: "routepass",
								},
							},
						},
					},
				}

				consumer1 := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer1",
						Security: &gatewayv1.ConsumeRouteSecurity{
							M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "consumer1user",
									Password: "consumer1pass",
								},
							},
						},
					},
				}

				consumer2 := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer2",
						Security: &gatewayv1.ConsumeRouteSecurity{
							M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
								// No basic auth for consumer2
							},
						},
					},
				}

				mockFeatureBuilder.EXPECT().JumperConfig().Return(jumperConfig)
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer1, consumer2})

				// Execute
				err := feature.InstanceBasicAuthFeature.Apply(context.Background(), mockFeatureBuilder)

				// Verify
				Expect(err).ToNot(HaveOccurred())

				// Check default provider creds
				defaultCreds := jumperConfig.BasicAuth[plugin.ConsumerId(feature.DefaultProviderKey)]
				Expect(defaultCreds.Username).To(Equal("routeuser"))
				Expect(defaultCreds.Password).To(Equal("routepass"))

				// Check consumer1 creds
				consumer1Creds := jumperConfig.BasicAuth[plugin.ConsumerId("consumer1")]
				Expect(consumer1Creds.Username).To(Equal("consumer1user"))
				Expect(consumer1Creds.Password).To(Equal("consumer1pass"))

				// Consumer2 should not have basic auth creds
				_, hasConsumer2Creds := jumperConfig.BasicAuth[plugin.ConsumerId("consumer2")]
				Expect(hasConsumer2Creds).To(BeFalse())
			})

			It("should apply basic auth credentials for consumer only configuration", func() {
				// Setup
				jumperConfig := plugin.NewJumperConfig()
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Security:    nil,
					},
				}

				consumer := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer",
						Security: &gatewayv1.ConsumeRouteSecurity{
							M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "consumeruser",
									Password: "consumerpass",
								},
							},
						},
					},
				}

				mockFeatureBuilder.EXPECT().JumperConfig().Return(jumperConfig)
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer})

				// Execute
				err := feature.InstanceBasicAuthFeature.Apply(context.Background(), mockFeatureBuilder)

				// Verify
				Expect(err).ToNot(HaveOccurred())

				// Check consumer creds
				consumerCreds := jumperConfig.BasicAuth[plugin.ConsumerId("consumer")]
				Expect(consumerCreds.Username).To(Equal("consumeruser"))
				Expect(consumerCreds.Password).To(Equal("consumerpass"))

				// Default provider should not have creds
				_, hasDefaultCreds := jumperConfig.BasicAuth[plugin.ConsumerId(feature.DefaultProviderKey)]
				Expect(hasDefaultCreds).To(BeFalse())
			})

			It("should skip consumers with nil security", func() {
				// Setup
				jumperConfig := plugin.NewJumperConfig()
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						PassThrough: false,
						Security: &gatewayv1.Security{
							M2M: &gatewayv1.Machine2MachineAuthentication{
								Basic: &gatewayv1.BasicAuthCredentials{
									Username: "routeuser",
									Password: "routepass",
								},
							},
						},
					},
				}

				consumer1 := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer1",
						Security:     nil, // Nil security
					},
				}

				mockFeatureBuilder.EXPECT().JumperConfig().Return(jumperConfig)
				mockFeatureBuilder.EXPECT().GetRoute().Return(route, true).AnyTimes()
				mockFeatureBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{consumer1})

				// Execute
				err := feature.InstanceBasicAuthFeature.Apply(context.Background(), mockFeatureBuilder)

				// Verify
				Expect(err).ToNot(HaveOccurred())

				// Check default provider creds
				defaultCreds := jumperConfig.BasicAuth[plugin.ConsumerId(feature.DefaultProviderKey)]
				Expect(defaultCreds.Username).To(Equal("routeuser"))
				Expect(defaultCreds.Password).To(Equal("routepass"))

				// Consumer1 should not have basic auth creds
				_, hasConsumer1Creds := jumperConfig.BasicAuth[plugin.ConsumerId("consumer1")]
				Expect(hasConsumer1Creds).To(BeFalse())
			})
		})
	})
})
