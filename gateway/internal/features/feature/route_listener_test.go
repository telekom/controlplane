// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ = Describe("RouteListenerFeature", func() {

	var (
		ctx     context.Context
		f       *feature.RouteListenerFeature
		builder *featmock.MockFeaturesBuilder
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = feature.InstanceRouteListenerFeature
		builder = featmock.NewMockFeaturesBuilder(GinkgoT())
	})

	Describe("Name()", func() {
		It("returns FeatureTypeRouteListener", func() {
			Expect(f.Name()).To(Equal(gatewayv1.FeatureTypeRouteListener))
		})
	})

	Describe("Priority()", func() {
		It("returns LastMileSecurity priority + 3", func() {
			Expect(f.Priority()).To(Equal(feature.InstanceLastMileSecurityFeature.Priority() + 3))
		})
	})

	Describe("IsUsed()", func() {
		Context("when there are no RouteListeners", func() {
			It("returns false", func() {
				builder.EXPECT().GetRouteListeners().Return(nil)

				Expect(f.IsUsed(ctx, builder)).To(BeFalse())
			})
		})

		Context("when there is one RouteListener", func() {
			It("returns true", func() {
				builder.EXPECT().GetRouteListeners().Return([]*gatewayv1.RouteListener{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "rl-1", Namespace: "test-ns"},
						Spec: gatewayv1.RouteListenerSpec{
							Consumer:     "consumer-app",
							ServiceOwner: "provider-app",
							Issue:        "/api/v1/events",
						},
					},
				})

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})

		Context("when there are multiple RouteListeners", func() {
			It("returns true", func() {
				builder.EXPECT().GetRouteListeners().Return([]*gatewayv1.RouteListener{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "rl-1", Namespace: "test-ns"},
						Spec: gatewayv1.RouteListenerSpec{
							Consumer:     "consumer-a",
							ServiceOwner: "provider-app",
							Issue:        "/api/v1/events",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "rl-2", Namespace: "test-ns"},
						Spec: gatewayv1.RouteListenerSpec{
							Consumer:     "consumer-b",
							ServiceOwner: "provider-app",
							Issue:        "/api/v2/events",
						},
					},
				})

				Expect(f.IsUsed(ctx, builder)).To(BeTrue())
			})
		})
	})

	Describe("Apply()", func() {
		Context("when there is one RouteListener", func() {
			It("populates jumper_config.routeListener map with consumer entry", func() {
				rl := &gatewayv1.RouteListener{
					ObjectMeta: metav1.ObjectMeta{Name: "rl-1", Namespace: "test-ns"},
					Spec: gatewayv1.RouteListenerSpec{
						Consumer:     "consumer-app",
						ServiceOwner: "provider-app",
						Issue:        "/api/v1/events",
					},
				}

				jc := plugin.NewJumperConfig()
				builder.EXPECT().GetRouteListeners().Return([]*gatewayv1.RouteListener{rl})
				builder.EXPECT().JumperConfig().Return(jc)
				builder.EXPECT().SetUpstream(mock.Anything).Return()

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())

				Expect(jc.RouteListener).To(HaveLen(1))
				Expect(jc.RouteListener).To(HaveKey(plugin.ConsumerId("consumer-app")))
				entry := jc.RouteListener[plugin.ConsumerId("consumer-app")]
				Expect(entry.Issue).To(Equal("/api/v1/events"))
				Expect(entry.ServiceOwner).To(Equal("provider-app"))
			})

			It("puts the publisher credentials in the top-level gatewayClient", func() {
				rl := &gatewayv1.RouteListener{
					ObjectMeta: metav1.ObjectMeta{Name: "rl-1", Namespace: "test-ns"},
					Spec: gatewayv1.RouteListenerSpec{
						Consumer:     "consumer-app",
						ServiceOwner: "provider-app",
						Issue:        "/api/v1/events",
						GatewayClient: gatewayv1.GatewayClientConfig{
							ClientId: "gateway",
							Issuer:   "https://iris.example.com/auth/realms/test",
						},
					},
				}

				jc := plugin.NewJumperConfig()
				builder.EXPECT().GetRouteListeners().Return([]*gatewayv1.RouteListener{rl})
				builder.EXPECT().JumperConfig().Return(jc)
				builder.EXPECT().SetUpstream(mock.Anything).Return()

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())

				Expect(jc.GatewayClient).ToNot(BeNil(), "jumper reads credentials from the top-level gatewayClient")
				Expect(jc.GatewayClient.Id).To(Equal("gateway"))
				Expect(jc.GatewayClient.Issuer).To(Equal("https://iris.example.com/auth/realms/test"))
				// Deferred: the secret must be resolved via secret-manager rather
				// than carried literally in a CR spec, so it is not populated yet.
				Expect(jc.GatewayClient.Secret).To(BeEmpty())
			})
		})

		Context("when there are multiple RouteListeners", func() {
			It("populates jumper_config.routeListener map with multiple entries", func() {
				rls := []*gatewayv1.RouteListener{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "rl-1", Namespace: "test-ns"},
						Spec: gatewayv1.RouteListenerSpec{
							Consumer:     "consumer-a",
							ServiceOwner: "provider-app",
							Issue:        "/api/v1/events",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "rl-2", Namespace: "test-ns"},
						Spec: gatewayv1.RouteListenerSpec{
							Consumer:     "consumer-b",
							ServiceOwner: "other-provider",
							Issue:        "/api/v2/notifications",
						},
					},
				}

				jc := plugin.NewJumperConfig()
				builder.EXPECT().GetRouteListeners().Return(rls)
				builder.EXPECT().JumperConfig().Return(jc)
				builder.EXPECT().SetUpstream(mock.Anything).Return()

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())

				Expect(jc.RouteListener).To(HaveLen(2))

				entryA := jc.RouteListener[plugin.ConsumerId("consumer-a")]
				Expect(entryA.Issue).To(Equal("/api/v1/events"))
				Expect(entryA.ServiceOwner).To(Equal("provider-app"))

				entryB := jc.RouteListener[plugin.ConsumerId("consumer-b")]
				Expect(entryB.Issue).To(Equal("/api/v2/notifications"))
				Expect(entryB.ServiceOwner).To(Equal("other-provider"))
			})
		})

		Context("when two RouteListeners target the same consumer", func() {
			It("returns an error naming the conflicting consumer instead of silently discarding one", func() {
				rls := []*gatewayv1.RouteListener{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "rl-b", Namespace: "test-ns"},
						Spec: gatewayv1.RouteListenerSpec{
							Consumer:     "consumer-shared",
							ServiceOwner: "provider-b",
							Issue:        "/api/v2/notifications",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: "rl-a", Namespace: "test-ns"},
						Spec: gatewayv1.RouteListenerSpec{
							Consumer:     "consumer-shared",
							ServiceOwner: "provider-a",
							Issue:        "/api/v1/events",
						},
					},
				}

				jc := plugin.NewJumperConfig()
				builder.EXPECT().GetRouteListeners().Return(rls)
				builder.EXPECT().JumperConfig().Return(jc)
				builder.EXPECT().SetUpstream(mock.Anything).Return().Maybe()

				err := f.Apply(ctx, builder)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer-shared"))
				Expect(err.Error()).To(ContainSubstring("/api/v1/events"))
				Expect(err.Error()).To(ContainSubstring("/api/v2/notifications"))
			})
		})

		Context("when RouteListener map is already initialized", func() {
			It("keeps pre-existing entries for other consumers", func() {
				rl := &gatewayv1.RouteListener{
					ObjectMeta: metav1.ObjectMeta{Name: "rl-new", Namespace: "test-ns"},
					Spec: gatewayv1.RouteListenerSpec{
						Consumer:     "consumer-new",
						ServiceOwner: "provider-new",
						Issue:        "/api/v3/data",
					},
				}

				jc := plugin.NewJumperConfig()
				jc.RouteListener = map[plugin.ConsumerId]plugin.RouteListenerEntry{
					"existing-consumer": {Issue: "/existing", ServiceOwner: "existing-provider"},
				}

				builder.EXPECT().GetRouteListeners().Return([]*gatewayv1.RouteListener{rl})
				builder.EXPECT().JumperConfig().Return(jc)
				builder.EXPECT().SetUpstream(mock.Anything).Return()

				err := f.Apply(ctx, builder)
				Expect(err).ToNot(HaveOccurred())

				Expect(jc.RouteListener).To(HaveLen(2))
				Expect(jc.RouteListener).To(HaveKey(plugin.ConsumerId("existing-consumer")))
				Expect(jc.RouteListener).To(HaveKey(plugin.ConsumerId("consumer-new")))
			})
		})
	})
})
