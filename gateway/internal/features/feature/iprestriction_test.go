// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IpRestrictionFeature", func() {
	var (
		ctx     context.Context
		f       *feature.IpRestrictionFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceIpRestrictionFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeIpRestriction", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeIpRestriction))
		})
	})

	Describe("Priority()", func() {
		It("returns 10", func() {
			Expect(f.Priority()).To(Equal(10))
		})
	})

	Describe("IsUsed()", func() {
		Context("when consumer has IP restrictions", func() {
			It("returns true", func() {
				consumer := &gatewayv1.Consumer{
					Spec: gatewayv1.ConsumerSpec{
						Security: &gatewayv1.ConsumerSecurity{
							IpRestrictions: &gatewayv1.IpRestrictions{
								Allow: []string{"10.0.0.1"},
							},
						},
					},
				}
				builder.EXPECT().GetConsumer().Return(consumer, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when consumer has no IP restrictions", func() {
			It("returns false", func() {
				consumer := &gatewayv1.Consumer{
					Spec: gatewayv1.ConsumerSpec{
						Security: nil,
					},
				}
				builder.EXPECT().GetConsumer().Return(consumer, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when no consumer in builder", func() {
			It("returns false", func() {
				builder.EXPECT().GetConsumer().Return(nil, false)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})
	})

	Describe("Apply()", func() {
		Context("happy path", func() {
			Context("when consumer has allow and deny lists", func() {
				It("populates IpRestrictionPlugin with allow and deny entries", func() {
					consumer := &gatewayv1.Consumer{
						Spec: gatewayv1.ConsumerSpec{
							Name: "test-consumer",
							Security: &gatewayv1.ConsumerSecurity{
								IpRestrictions: &gatewayv1.IpRestrictions{
									Allow: []string{"10.0.0.1", "172.16.0.0/12"},
									Deny:  []string{"192.168.1.0/24", "10.10.10.10"},
								},
							},
						},
					}
					ipPlugin := plugin.IpRestrictionPluginFromConsumer(consumer)

					builder.EXPECT().GetConsumer().Return(consumer, true)
					builder.EXPECT().IpRestrictionPlugin().Return(ipPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(ipPlugin.Config.Allow.Contains("10.0.0.1")).To(BeTrue())
					Expect(ipPlugin.Config.Allow.Contains("172.16.0.0/12")).To(BeTrue())
					Expect(ipPlugin.Config.Deny.Contains("192.168.1.0/24")).To(BeTrue())
					Expect(ipPlugin.Config.Deny.Contains("10.10.10.10")).To(BeTrue())
				})
			})
		})

		Context("error handling", func() {
			Context("when no consumer in builder", func() {
				It("returns ErrNoConsumer", func() {
					builder.EXPECT().GetConsumer().Return(nil, false)

					err := f.Apply(ctx, builder)
					Expect(err).To(MatchError(features.ErrNoConsumer))
				})
			})
		})

		Context("edge cases", func() {
			Context("when only allow list", func() {
				It("populates only allow entries", func() {
					consumer := &gatewayv1.Consumer{
						Spec: gatewayv1.ConsumerSpec{
							Name: "test-consumer",
							Security: &gatewayv1.ConsumerSecurity{
								IpRestrictions: &gatewayv1.IpRestrictions{
									Allow: []string{"10.0.0.1"},
								},
							},
						},
					}
					ipPlugin := plugin.IpRestrictionPluginFromConsumer(consumer)

					builder.EXPECT().GetConsumer().Return(consumer, true)
					builder.EXPECT().IpRestrictionPlugin().Return(ipPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(ipPlugin.Config.Allow.Contains("10.0.0.1")).To(BeTrue())
					Expect(ipPlugin.Config.Deny.Empty()).To(BeTrue())
				})
			})

			Context("when only deny list", func() {
				It("populates only deny entries", func() {
					consumer := &gatewayv1.Consumer{
						Spec: gatewayv1.ConsumerSpec{
							Name: "test-consumer",
							Security: &gatewayv1.ConsumerSecurity{
								IpRestrictions: &gatewayv1.IpRestrictions{
									Deny: []string{"192.168.1.0/24"},
								},
							},
						},
					}
					ipPlugin := plugin.IpRestrictionPluginFromConsumer(consumer)

					builder.EXPECT().GetConsumer().Return(consumer, true)
					builder.EXPECT().IpRestrictionPlugin().Return(ipPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(ipPlugin.Config.Allow.Empty()).To(BeTrue())
					Expect(ipPlugin.Config.Deny.Contains("192.168.1.0/24")).To(BeTrue())
				})
			})
		})
	})
})
