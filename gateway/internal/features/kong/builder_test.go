// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kong_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/kong"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	clientmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
)

var _ = Describe("Builder", func() {
	var (
		ctx     context.Context
		mockKC  *clientmock.MockKongClient
		route   *gatewayv1.Route
		gateway *gatewayv1.Gateway
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockKC = clientmock.NewMockKongClient(GinkgoT())
		route = &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "default",
			},
			Spec: gatewayv1.RouteSpec{
				Hostnames: []string{"example.com"},
				Paths:     []string{"/api"},
			},
		}
		gateway = &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gateway",
				Namespace: "default",
			},
		}
	})

	Describe("Build()", func() {
		Context("happy path", func() {
			It("sorts and applies features, creates route+plugins, and calls cleanup", func() {
				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("test-feature")).Maybe()
				mockFeature.EXPECT().Priority().Return(10).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockKC.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).Return(nil, nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				// Trigger plugin creation so at least one plugin exists
				builder.RequestTransformerPlugin()

				err := builder.Build(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when a feature returns IsUsed=false", func() {
			It("skips that feature without calling Apply", func() {
				skippedFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				skippedFeature.EXPECT().Name().Return(gatewayv1.FeatureType("skipped")).Maybe()
				skippedFeature.EXPECT().Priority().Return(5).Maybe()
				skippedFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(false)
				// Apply should NOT be called

				usedFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				usedFeature.EXPECT().Name().Return(gatewayv1.FeatureType("used")).Maybe()
				usedFeature.EXPECT().Priority().Return(10).Maybe()
				usedFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				usedFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(skippedFeature)
				builder.EnableFeature(usedFeature)

				err := builder.Build(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when a feature Apply returns an error", func() {
			It("propagates the error and stops the pipeline", func() {
				applyErr := errors.New("feature apply failed")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("failing")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).Return(applyErr)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).To(MatchError(applyErr))
			})
		})

		Context("when no upstream is set after features", func() {
			It("returns an error indicating upstream is not set", func() {
				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("no-upstream")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil)
				// Feature does NOT set an upstream

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("upstream is not set"))
			})
		})

		Context("when route is nil", func() {
			It("panics due to nil dereference before the guard check", func() {
				// NOTE: Build() accesses b.Route.Name for logging before the nil check,
				// so a nil route causes a panic rather than returning ErrNoRoute.
				builder := kong.NewFeatureBuilder(mockKC, nil, nil, gateway)
				Expect(func() {
					_ = builder.Build(ctx) //nolint:errcheck
				}).To(Panic())
			})
		})

		Context("when RoutingConfigs are present", func() {
			It("adds routing_config header to the request transformer plugin", func() {
				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("routing")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
						fb.RoutingConfigs().Add(&plugin.RoutingConfig{
							RemoteApiUrl: "http://remote.example.com",
						})
					}).Return(nil)

				var createdPlugins []client.CustomPlugin
				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockKC.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).
					Run(func(_ context.Context, p client.CustomPlugin) {
						createdPlugins = append(createdPlugins, p)
					}).Return(nil, nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(createdPlugins).To(HaveLen(1))

				rtpPlugin := createdPlugins[0].(*plugin.RequestTransformerPlugin)
				Expect(rtpPlugin.GetName()).To(Equal("request-transformer"))
				cfg := rtpPlugin.GetConfig()
				appendCfg := cfg["append"]
				Expect(appendCfg).ToNot(BeNil())
			})
		})

		Context("when JumperConfig is present (without RoutingConfigs)", func() {
			It("adds jumper_config header to the request transformer plugin", func() {
				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("jumper")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
						// Access JumperConfig to initialize it (simulating a feature that sets it)
						fb.JumperConfig().OAuth["test-consumer"] = plugin.OauthCredentials{
							ClientId: "test-client",
						}
					}).Return(nil)

				var createdPlugins []client.CustomPlugin
				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockKC.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).
					Run(func(_ context.Context, p client.CustomPlugin) {
						createdPlugins = append(createdPlugins, p)
					}).Return(nil, nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(createdPlugins).To(HaveLen(1))

				rtpPlugin := createdPlugins[0].(*plugin.RequestTransformerPlugin)
				Expect(rtpPlugin.GetName()).To(Equal("request-transformer"))
			})
		})

		Context("when neither RoutingConfigs nor JumperConfig are present", func() {
			It("does not add routing or jumper headers", func() {
				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("plain")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when CreateOrReplaceRoute fails", func() {
			It("returns a wrapped error", func() {
				routeErr := errors.New("kong route creation failed")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("upstream-setter")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(routeErr)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create or replace route"))
				Expect(err.Error()).To(ContainSubstring("kong route creation failed"))
			})
		})

		Context("when CreateOrReplacePlugin fails", func() {
			It("returns a wrapped error", func() {
				pluginErr := errors.New("kong plugin creation failed")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("plugin-fail")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
						// Access a plugin to ensure it's created in the Plugins map
						fb.RequestTransformerPlugin()
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockKC.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).Return(nil, pluginErr)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create or replace plugin"))
				Expect(err.Error()).To(ContainSubstring("kong plugin creation failed"))
			})
		})

		Context("when CleanupPlugins fails", func() {
			It("returns a wrapped error", func() {
				cleanupErr := errors.New("cleanup failed")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("cleanup-fail")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(cleanupErr)

				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.Build(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to cleanup plugins"))
				Expect(err.Error()).To(ContainSubstring("cleanup failed"))
			})
		})
	})

	Describe("BuildForConsumer()", func() {
		var consumer *gatewayv1.Consumer

		BeforeEach(func() {
			consumer = &gatewayv1.Consumer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-consumer",
					Namespace: "default",
				},
				Spec: gatewayv1.ConsumerSpec{
					Name: "test-consumer-spec",
				},
			}
		})

		Context("happy path", func() {
			It("applies features, creates consumer and plugins, and calls cleanup", func() {
				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("consumer-feature")).Maybe()
				mockFeature.EXPECT().Priority().Return(10).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil)

				mockKC.EXPECT().CreateOrReplaceConsumer(mock.Anything, mock.Anything).Return(nil, nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.BuildForConsumer(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("happy path with plugins", func() {
			It("creates plugins via CreateOrReplacePlugin", func() {
				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("ip-feature")).Maybe()
				mockFeature.EXPECT().Priority().Return(10).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.IpRestrictionPlugin()
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceConsumer(mock.Anything, mock.Anything).Return(nil, nil)
				mockKC.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).Return(nil, nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.BuildForConsumer(ctx)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when consumer is nil", func() {
			It("panics due to nil dereference before the guard check", func() {
				// NOTE: BuildForConsumer() accesses b.Consumer.Name for logging
				// before the nil check, so a nil consumer causes a panic.
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				Expect(func() {
					_ = builder.BuildForConsumer(ctx) //nolint:errcheck
				}).To(Panic())
			})
		})

		Context("when a feature Apply returns an error", func() {
			It("propagates the error", func() {
				applyErr := errors.New("consumer feature error")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("failing-consumer")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).Return(applyErr)

				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.BuildForConsumer(ctx)
				Expect(err).To(MatchError(applyErr))
			})
		})

		Context("when CreateOrReplaceConsumer fails", func() {
			It("returns a wrapped error", func() {
				consumerErr := errors.New("kong consumer creation failed")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("consumer-ok")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil)

				mockKC.EXPECT().CreateOrReplaceConsumer(mock.Anything, mock.Anything).Return(nil, consumerErr)

				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.BuildForConsumer(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create or replace consumer"))
				Expect(err.Error()).To(ContainSubstring("kong consumer creation failed"))
			})
		})

		Context("when CreateOrReplacePlugin fails", func() {
			It("returns a wrapped error", func() {
				pluginErr := errors.New("consumer plugin creation failed")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("consumer-plugin-fail")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
					Run(func(_ context.Context, fb features.KongFeatureBuilder) {
						fb.IpRestrictionPlugin()
					}).Return(nil)

				mockKC.EXPECT().CreateOrReplaceConsumer(mock.Anything, mock.Anything).Return(nil, nil)
				mockKC.EXPECT().CreateOrReplacePlugin(mock.Anything, mock.Anything).Return(nil, pluginErr)

				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.BuildForConsumer(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create or replace plugin"))
				Expect(err.Error()).To(ContainSubstring("consumer plugin creation failed"))
			})
		})

		Context("when CleanupPlugins fails", func() {
			It("returns a wrapped error", func() {
				cleanupErr := errors.New("consumer cleanup failed")

				mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
				mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("consumer-cleanup")).Maybe()
				mockFeature.EXPECT().Priority().Return(1).Maybe()
				mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
				mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).Return(nil)

				mockKC.EXPECT().CreateOrReplaceConsumer(mock.Anything, mock.Anything).Return(nil, nil)
				mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(cleanupErr)

				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				builder.EnableFeature(mockFeature)

				err := builder.BuildForConsumer(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to cleanup plugins"))
				Expect(err.Error()).To(ContainSubstring("consumer cleanup failed"))
			})
		})
	})

	Describe("Feature ordering", func() {
		It("applies features in ascending priority order (lowest first)", func() {
			var appliedOrder []int

			f0 := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
			f0.EXPECT().Name().Return(gatewayv1.FeatureType("feature-10")).Maybe()
			f0.EXPECT().Priority().Return(10).Maybe()
			f0.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
			f0.EXPECT().Apply(mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ features.KongFeatureBuilder) {
					appliedOrder = append(appliedOrder, 10)
				}).Return(nil)

			f1 := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
			f1.EXPECT().Name().Return(gatewayv1.FeatureType("feature-0")).Maybe()
			f1.EXPECT().Priority().Return(0).Maybe()
			f1.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
			f1.EXPECT().Apply(mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ features.KongFeatureBuilder) {
					appliedOrder = append(appliedOrder, 0)
				}).Return(nil)

			f2 := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
			f2.EXPECT().Name().Return(gatewayv1.FeatureType("feature-100")).Maybe()
			f2.EXPECT().Priority().Return(100).Maybe()
			f2.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
			f2.EXPECT().Apply(mock.Anything, mock.Anything).
				Run(func(_ context.Context, fb features.KongFeatureBuilder) {
					appliedOrder = append(appliedOrder, 100)
					fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
				}).Return(nil)

			mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			builder.EnableFeature(f0)
			builder.EnableFeature(f1)
			builder.EnableFeature(f2)

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(appliedOrder).To(Equal([]int{0, 10, 100}))
		})
	})

	Describe("EnableFeature()", func() {
		It("adds the feature to the builder", func() {
			mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
			mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("my-feature")).Maybe()
			mockFeature.EXPECT().Priority().Return(5).Maybe()
			mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
			mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
				Run(func(_ context.Context, fb features.KongFeatureBuilder) {
					fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
				}).Return(nil)

			mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			builder.EnableFeature(mockFeature)

			// The feature is used during Build, confirming it was registered
			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("replaces a feature with the same name", func() {
			firstFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
			firstFeature.EXPECT().Name().Return(gatewayv1.FeatureType("duplicate")).Maybe()
			firstFeature.EXPECT().Priority().Return(1).Maybe()
			// The first feature should NOT be called (replaced by second)

			secondFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
			secondFeature.EXPECT().Name().Return(gatewayv1.FeatureType("duplicate")).Maybe()
			secondFeature.EXPECT().Priority().Return(2).Maybe()
			secondFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
			secondFeature.EXPECT().Apply(mock.Anything, mock.Anything).
				Run(func(_ context.Context, fb features.KongFeatureBuilder) {
					fb.SetUpstream(client.NewUpstreamOrDie(plugin.LocalhostProxyUrl))
				}).Return(nil)

			mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			builder.EnableFeature(firstFeature)
			builder.EnableFeature(secondFeature) // Should replace the first

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Plugin getters (lazy initialization)", func() {
		Context("RequestTransformerPlugin()", func() {
			It("creates a new plugin on first call", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rtp := builder.RequestTransformerPlugin()
				Expect(rtp).ToNot(BeNil())
				Expect(rtp.GetName()).To(Equal("request-transformer"))
			})

			It("returns the same plugin on subsequent calls", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rtp1 := builder.RequestTransformerPlugin()
				rtp2 := builder.RequestTransformerPlugin()
				Expect(rtp1).To(BeIdenticalTo(rtp2))
			})
		})

		Context("AclPlugin()", func() {
			It("creates a new plugin on first call", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				acl := builder.AclPlugin()
				Expect(acl).ToNot(BeNil())
				Expect(acl.GetName()).To(Equal("acl"))
			})

			It("returns the same plugin on subsequent calls", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				acl1 := builder.AclPlugin()
				acl2 := builder.AclPlugin()
				Expect(acl1).To(BeIdenticalTo(acl2))
			})
		})

		Context("JwtPlugin()", func() {
			It("creates a new plugin on first call", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				jwt := builder.JwtPlugin()
				Expect(jwt).ToNot(BeNil())
				Expect(jwt.GetName()).To(Equal("jwt-keycloak"))
			})

			It("returns the same plugin on subsequent calls", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				jwt1 := builder.JwtPlugin()
				jwt2 := builder.JwtPlugin()
				Expect(jwt1).To(BeIdenticalTo(jwt2))
			})
		})

		Context("RateLimitPluginRoute()", func() {
			It("creates a new plugin on first call", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rl := builder.RateLimitPluginRoute()
				Expect(rl).ToNot(BeNil())
				Expect(rl.GetName()).To(Equal("rate-limiting-merged"))
			})

			It("returns the same plugin on subsequent calls", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rl1 := builder.RateLimitPluginRoute()
				rl2 := builder.RateLimitPluginRoute()
				Expect(rl1).To(BeIdenticalTo(rl2))
			})
		})

		Context("RateLimitPluginConsumeRoute()", func() {
			It("creates a new plugin keyed by consumer name", func() {
				cr := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer-a",
					},
				}
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rl := builder.RateLimitPluginConsumeRoute(cr)
				Expect(rl).ToNot(BeNil())
				Expect(rl.GetName()).To(Equal("rate-limiting-merged"))
			})

			It("returns the same plugin for the same consumer", func() {
				cr := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer-b",
					},
				}
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rl1 := builder.RateLimitPluginConsumeRoute(cr)
				rl2 := builder.RateLimitPluginConsumeRoute(cr)
				Expect(rl1).To(BeIdenticalTo(rl2))
			})

			It("returns different plugins for different consumers", func() {
				crA := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer-x",
					},
				}
				crB := &gatewayv1.ConsumeRoute{
					Spec: gatewayv1.ConsumeRouteSpec{
						ConsumerName: "consumer-y",
					},
				}
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rlA := builder.RateLimitPluginConsumeRoute(crA)
				rlB := builder.RateLimitPluginConsumeRoute(crB)
				Expect(rlA).ToNot(BeIdenticalTo(rlB))
			})
		})

		Context("IpRestrictionPlugin()", func() {
			It("creates a new plugin on first call", func() {
				consumer := &gatewayv1.Consumer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-consumer",
						Namespace: "default",
					},
				}
				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				ip := builder.IpRestrictionPlugin()
				Expect(ip).ToNot(BeNil())
				Expect(ip.GetName()).To(Equal("ip-restriction"))
			})

			It("returns the same plugin on subsequent calls", func() {
				consumer := &gatewayv1.Consumer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-consumer",
						Namespace: "default",
					},
				}
				builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
				ip1 := builder.IpRestrictionPlugin()
				ip2 := builder.IpRestrictionPlugin()
				Expect(ip1).To(BeIdenticalTo(ip2))
			})
		})

		Context("JumperConfig()", func() {
			It("creates a new config on first call", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				jc := builder.JumperConfig()
				Expect(jc).ToNot(BeNil())
				Expect(jc.OAuth).ToNot(BeNil())
				Expect(jc.BasicAuth).ToNot(BeNil())
			})

			It("returns the same config on subsequent calls", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				jc1 := builder.JumperConfig()
				jc2 := builder.JumperConfig()
				Expect(jc1).To(BeIdenticalTo(jc2))
			})
		})

		Context("RoutingConfigs()", func() {
			It("creates a new config on first call", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rc := builder.RoutingConfigs()
				Expect(rc).ToNot(BeNil())
				Expect(rc.Len()).To(Equal(0))
			})

			It("returns the same config on subsequent calls", func() {
				builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
				rc1 := builder.RoutingConfigs()
				rc2 := builder.RoutingConfigs()
				Expect(rc1).To(BeIdenticalTo(rc2))
			})
		})
	})

	Describe("SetUpstream()", func() {
		It("stores the upstream for use during Build", func() {
			upstream := client.NewUpstreamOrDie("http://my-service:8080/path")

			mockFeature := featmock.NewMockFeature[features.KongFeatureBuilder](GinkgoT())
			mockFeature.EXPECT().Name().Return(gatewayv1.FeatureType("noop")).Maybe()
			mockFeature.EXPECT().Priority().Return(1).Maybe()
			mockFeature.EXPECT().IsUsed(mock.Anything, mock.Anything).Return(true)
			mockFeature.EXPECT().Apply(mock.Anything, mock.Anything).
				Run(func(_ context.Context, fb features.KongFeatureBuilder) {
					fb.SetUpstream(upstream)
				}).Return(nil)

			var receivedUpstream client.Upstream
			mockKC.EXPECT().CreateOrReplaceRoute(mock.Anything, mock.Anything, mock.Anything).
				Run(func(_ context.Context, _ client.CustomRoute, u client.Upstream) {
					receivedUpstream = u
				}).Return(nil)
			mockKC.EXPECT().CleanupPlugins(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			builder.EnableFeature(mockFeature)

			err := builder.Build(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(receivedUpstream).To(Equal(upstream))
		})
	})

	Describe("GetRoute()", func() {
		It("returns the route and true when set", func() {
			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			r, ok := builder.GetRoute()
			Expect(ok).To(BeTrue())
			Expect(r).To(Equal(route))
		})

		It("returns nil and false when route is not set", func() {
			builder := kong.NewFeatureBuilder(mockKC, nil, nil, gateway)
			r, ok := builder.GetRoute()
			Expect(ok).To(BeFalse())
			Expect(r).To(BeNil())
		})
	})

	Describe("GetConsumer()", func() {
		It("returns the consumer and true when set", func() {
			consumer := &gatewayv1.Consumer{
				ObjectMeta: metav1.ObjectMeta{Name: "my-consumer"},
			}
			builder := kong.NewFeatureBuilder(mockKC, nil, consumer, gateway)
			c, ok := builder.GetConsumer()
			Expect(ok).To(BeTrue())
			Expect(c).To(Equal(consumer))
		})

		It("returns nil and false when consumer is not set", func() {
			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			c, ok := builder.GetConsumer()
			Expect(ok).To(BeFalse())
			Expect(c).To(BeNil())
		})
	})

	Describe("GetGateway()", func() {
		It("returns the gateway", func() {
			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			Expect(builder.GetGateway()).To(Equal(gateway))
		})
	})

	Describe("GetAllowedConsumers() / AddAllowedConsumers()", func() {
		It("starts empty", func() {
			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			Expect(builder.GetAllowedConsumers()).To(BeEmpty())
		})

		It("appends consumers", func() {
			cr1 := &gatewayv1.ConsumeRoute{
				Spec: gatewayv1.ConsumeRouteSpec{ConsumerName: "c1"},
			}
			cr2 := &gatewayv1.ConsumeRoute{
				Spec: gatewayv1.ConsumeRouteSpec{ConsumerName: "c2"},
			}

			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			builder.AddAllowedConsumers(cr1, cr2)
			Expect(builder.GetAllowedConsumers()).To(HaveLen(2))
			Expect(builder.GetAllowedConsumers()).To(ContainElements(cr1, cr2))
		})
	})

	Describe("GetKongClient()", func() {
		It("returns the kong client", func() {
			builder := kong.NewFeatureBuilder(mockKC, route, nil, gateway)
			Expect(builder.GetKongClient()).To(Equal(mockKC))
		})
	})
})
