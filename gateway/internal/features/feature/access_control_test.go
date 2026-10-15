// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AccessControlFeature", func() {
	var (
		ctx     context.Context
		f       *feature.AccessControlFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceAccessControlFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeAccessControl", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeAccessControl))
		})
	})

	Describe("Priority()", func() {
		It("returns 10", func() {
			Expect(f.Priority()).To(Equal(10))
		})
	})

	Describe("IsUsed()", func() {
		Context("when route has trusted issuers", func() {
			It("returns true", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Security: gatewayv1.Security{
							TrustedIssuers: []string{"https://issuer.example.com"},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when route has no trusted issuers", func() {
			It("returns false", func() {
				route := &gatewayv1.Route{
					Spec: gatewayv1.RouteSpec{
						Security: gatewayv1.Security{
							TrustedIssuers: []string{},
						},
					},
				}
				builder.EXPECT().GetRoute().Return(route, true)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when no route in builder", func() {
			It("returns false", func() {
				builder.EXPECT().GetRoute().Return(nil, false)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})
	})

	Describe("Apply()", func() {
		Context("happy path", func() {
			Context("when route has issuers and allowed consumers", func() {
				It("initializes JWT plugin and populates ACL with consumer names", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{
								TrustedIssuers: []string{"https://issuer.example.com"},
							},
						},
					}

					aclPlugin := plugin.AclPluginFromRoute(route)
					jwtPlugin := plugin.JwtPluginFromRoute(route)

					consumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
								ConsumerName: "consumer-a",
							},
						},
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
								ConsumerName: "consumer-b",
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JwtPlugin().Return(jwtPlugin)
					builder.EXPECT().AclPlugin().Return(aclPlugin)
					builder.EXPECT().GetAllowedConsumers().Return(consumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(aclPlugin.Config.Allow.Contains("consumer-a")).To(BeTrue())
					Expect(aclPlugin.Config.Allow.Contains("consumer-b")).To(BeTrue())
					Expect(aclPlugin.Config.Allow.Contains(plugin.DenyAllGroup)).To(BeFalse())
				})
			})

			Context("when route has default consumers", func() {
				It("adds default consumers to ACL allow list", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{
								TrustedIssuers:   []string{"https://issuer.example.com"},
								DefaultConsumers: []string{"default-consumer-a", "default-consumer-b"},
							},
						},
					}

					aclPlugin := plugin.AclPluginFromRoute(route)
					jwtPlugin := plugin.JwtPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JwtPlugin().Return(jwtPlugin)
					builder.EXPECT().AclPlugin().Return(aclPlugin)
					builder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(aclPlugin.Config.Allow.Contains("default-consumer-a")).To(BeTrue())
					Expect(aclPlugin.Config.Allow.Contains("default-consumer-b")).To(BeTrue())
					Expect(aclPlugin.Config.Allow.Contains(plugin.DenyAllGroup)).To(BeFalse())
				})
			})

			Context("when DisableAccessControl is true", func() {
				It("initializes JWT plugin but does not populate ACL", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{
								TrustedIssuers:       []string{"https://issuer.example.com"},
								DisableAccessControl: true,
							},
						},
					}

					jwtPlugin := plugin.JwtPluginFromRoute(route)

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JwtPlugin().Return(jwtPlugin)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		Context("error handling", func() {
			Context("when no route in builder", func() {
				It("returns ErrNoRoute", func() {
					builder.EXPECT().GetRoute().Return(nil, false)

					err := f.Apply(ctx, builder)
					Expect(err).To(MatchError(features.ErrNoRoute))
				})
			})
		})

		Context("edge cases", func() {
			Context("when no consumers match the route", func() {
				It("uses DenyAllGroup sentinel in ACL", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{
								TrustedIssuers: []string{"https://issuer.example.com"},
							},
						},
					}

					aclPlugin := plugin.AclPluginFromRoute(route)
					jwtPlugin := plugin.JwtPluginFromRoute(route)

					nonMatchingConsumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								Route:        types.ObjectRef{Name: "other-route", Namespace: "other-ns"},
								ConsumerName: "consumer-x",
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JwtPlugin().Return(jwtPlugin)
					builder.EXPECT().AclPlugin().Return(aclPlugin)
					builder.EXPECT().GetAllowedConsumers().Return(nonMatchingConsumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(aclPlugin.Config.Allow.Contains(plugin.DenyAllGroup)).To(BeTrue())
					Expect(aclPlugin.Config.Allow.Contains("consumer-x")).To(BeFalse())
				})
			})

			Context("when some consumers reference a different route", func() {
				It("only includes matching consumers", func() {
					route := &gatewayv1.Route{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-route",
							Namespace: "test-ns",
						},
						Spec: gatewayv1.RouteSpec{
							Security: gatewayv1.Security{
								TrustedIssuers: []string{"https://issuer.example.com"},
							},
						},
					}

					aclPlugin := plugin.AclPluginFromRoute(route)
					jwtPlugin := plugin.JwtPluginFromRoute(route)

					mixedConsumers := []*gatewayv1.ConsumeRoute{
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
								ConsumerName: "matching-consumer",
							},
						},
						{
							Spec: gatewayv1.ConsumeRouteSpec{
								Route:        types.ObjectRef{Name: "other-route", Namespace: "other-ns"},
								ConsumerName: "non-matching-consumer",
							},
						},
					}

					builder.EXPECT().GetRoute().Return(route, true)
					builder.EXPECT().JwtPlugin().Return(jwtPlugin)
					builder.EXPECT().AclPlugin().Return(aclPlugin)
					builder.EXPECT().GetAllowedConsumers().Return(mixedConsumers)

					err := f.Apply(ctx, builder)
					Expect(err).ToNot(HaveOccurred())

					Expect(aclPlugin.Config.Allow.Contains("matching-consumer")).To(BeTrue())
					Expect(aclPlugin.Config.Allow.Contains("non-matching-consumer")).To(BeFalse())
					Expect(aclPlugin.Config.Allow.Contains(plugin.DenyAllGroup)).To(BeFalse())
				})
			})
		})
	})
})
