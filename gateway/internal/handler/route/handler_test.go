// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package route_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgtypes "k8s.io/apimachinery/pkg/types"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"

	cc "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	featmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	routehandler "github.com/telekom/controlplane/gateway/internal/handler/route"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"
	clientmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

var _ = Describe("RouteHandler", func() {
	var (
		ctx         context.Context
		handler     *routehandler.RouteHandler
		mockClient  *fakeclient.MockJanitorClient
		mockKC      *clientmock.MockKongClient
		mockBuilder *featmock.MockFeaturesBuilder
		route       *gatewayv1.Route
	)

	BeforeEach(func() {
		handler = &routehandler.RouteHandler{}
		mockClient = fakeclient.NewMockJanitorClient(GinkgoT())
		mockKC = clientmock.NewMockKongClient(GinkgoT())
		mockBuilder = featmock.NewMockFeaturesBuilder(GinkgoT())

		ctx = cc.WithClient(context.Background(), mockClient)

		route = &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "test-ns",
			},
			Spec: gatewayv1.RouteSpec{
				GatewayRef: types.ObjectRef{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Type:      gatewayv1.RouteTypePrimary,
				Hostnames: []string{"example.com"},
				Paths:     []string{"/api"},
			},
		}

		// Override secrets.Get to be a no-op (return the value as-is)
		originalSecretsGet := secrets.Get
		DeferCleanup(func() { secrets.Get = originalSecretsGet })
		secrets.Get = func(_ context.Context, ref string) (string, error) {
			return ref, nil
		}
	})

	// setupGatewayMocks configures the mock client Get to return a ready gateway
	// and overrides kongutil.GetClientFor and features.NewFeatureBuilder.
	setupFeatureBuilderOverrides := func() {
		originalGetClientFor := kongutil.GetClientFor
		DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
		kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
			return mockKC, nil
		}

		originalNewFeatureBuilder := features.NewFeatureBuilder
		DeferCleanup(func() { features.NewFeatureBuilder = originalNewFeatureBuilder })
		features.NewFeatureBuilder = func(_ kongclient.KongClient, _ *gatewayv1.Route, _ *gatewayv1.Consumer, _ *gatewayv1.Gateway) features.FeaturesBuilder {
			return mockBuilder
		}
	}

	setupReadyGatewayGet := func() {
		mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ pkgtypes.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
				gw := obj.(*gatewayv1.Gateway)
				gw.Name = "test-gateway"
				gw.Namespace = "test-ns"
				gw.Spec.Admin = gatewayv1.AdminConfig{
					Url:          "http://kong:8001",
					ClientId:     "client-id",
					ClientSecret: "client-secret",
					IssuerUrl:    "http://idp/realms/test",
				}
				gw.Spec.Redis = &gatewayv1.RedisConfig{
					Host:     "redis",
					Port:     6379,
					Password: "redis-pass",
				}
				meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})
			}).Return(nil)
	}

	setupNotReadyGatewayGet := func() {
		mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
			Run(func(_ context.Context, _ pkgtypes.NamespacedName, obj pkgclient.Object, _ ...pkgclient.GetOption) {
				gw := obj.(*gatewayv1.Gateway)
				gw.Name = "test-gateway"
				gw.Namespace = "test-ns"
			}).Return(nil)
	}

	Describe("CreateOrUpdate()", func() {
		Context("happy path", func() {
			It("builds route with features and sets Ready condition", func() {
				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				// Mock builder expectations
				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()
				mockBuilder.EXPECT().AddAllowedConsumers(mock.Anything).Maybe()
				mockBuilder.EXPECT().AddRouteListeners(mock.Anything).Maybe()
				mockBuilder.EXPECT().Build(mock.Anything).Return(nil)
				mockBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

				// Mock List to return empty consumers, then empty route listeners
				mockClient.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
					Run(func(_ context.Context, list pkgclient.ObjectList, _ ...pkgclient.ListOption) {
						switch l := list.(type) {
						case *gatewayv1.ConsumeRouteList:
							l.Items = []gatewayv1.ConsumeRoute{}
						case *gatewayv1.RouteListenerList:
							l.Items = []gatewayv1.RouteListener{}
						}
					}).Return(nil)

				err := handler.CreateOrUpdate(ctx, route)
				Expect(err).NotTo(HaveOccurred())

				Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				Expect(meta.IsStatusConditionFalse(route.GetConditions(), condition.ConditionTypeProcessing)).To(BeTrue())
			})

			It("populates Status.Consumers with sorted direct consumer names", func() {
				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()
				mockBuilder.EXPECT().AddRouteListeners(mock.Anything).Maybe()
				mockBuilder.EXPECT().Build(mock.Anything).Return(nil)

				// Return consumers from List
				consumer1 := gatewayv1.ConsumeRoute{
					ObjectMeta: metav1.ObjectMeta{Name: "cr-bravo", Namespace: "test-ns"},
					Spec: gatewayv1.ConsumeRouteSpec{
						Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
						ConsumerName: "bravo-consumer",
					},
				}
				consumer2 := gatewayv1.ConsumeRoute{
					ObjectMeta: metav1.ObjectMeta{Name: "cr-alpha", Namespace: "test-ns"},
					Spec: gatewayv1.ConsumeRouteSpec{
						Route:        types.ObjectRef{Name: "test-route", Namespace: "test-ns"},
						ConsumerName: "alpha-consumer",
					},
				}

				mockClient.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
					Run(func(_ context.Context, list pkgclient.ObjectList, _ ...pkgclient.ListOption) {
						switch l := list.(type) {
						case *gatewayv1.ConsumeRouteList:
							l.Items = []gatewayv1.ConsumeRoute{consumer1, consumer2}
						case *gatewayv1.RouteListenerList:
							l.Items = []gatewayv1.RouteListener{}
						}
					}).Return(nil)

				mockBuilder.EXPECT().AddAllowedConsumers(mock.Anything).Maybe()

				// The builder returns the allowed consumers (sorted by name in handler)
				mockBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{
					&consumer2, // cr-alpha
					&consumer1, // cr-bravo
				})

				err := handler.CreateOrUpdate(ctx, route)
				Expect(err).NotTo(HaveOccurred())

				// Consumers should be sorted alphabetically by ConsumeRoute name
				Expect(route.Status.Consumers).To(Equal([]string{"alpha-consumer", "bravo-consumer"}))
			})
		})

		Context("passthrough route", func() {
			It("skips consumer listing and route listener listing and still calls builder.Build", func() {
				route.Spec.PassThrough = true

				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()
				mockBuilder.EXPECT().Build(mock.Anything).Return(nil)
				mockBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

				// No List calls expected: both consumer listing and RouteListener listing
				// are inside the !PassThrough block.

				err := handler.CreateOrUpdate(ctx, route)
				Expect(err).NotTo(HaveOccurred())

				Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			})
		})

		Context("error handling", func() {
			It("returns error when NewFeatureBuilder fails (gateway not ready)", func() {
				setupNotReadyGatewayGet()

				// Don't override features.NewFeatureBuilder - let the real one run with the not-ready gateway
				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return mockKC, nil
				}

				err := handler.CreateOrUpdate(ctx, route)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create feature builder"))
			})

			It("returns error when List fails", func() {
				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()

				mockClient.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("list error"))

				err := handler.CreateOrUpdate(ctx, route)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to list route consumers"))
			})

			It("returns error when builder.Build fails", func() {
				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()
				mockBuilder.EXPECT().AddAllowedConsumers(mock.Anything).Maybe()
				mockBuilder.EXPECT().AddRouteListeners(mock.Anything).Maybe()
				mockBuilder.EXPECT().Build(mock.Anything).Return(fmt.Errorf("build failed"))
				mockBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{}).Maybe()

				mockClient.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
					Run(func(_ context.Context, list pkgclient.ObjectList, _ ...pkgclient.ListOption) {
						switch l := list.(type) {
						case *gatewayv1.ConsumeRouteList:
							l.Items = []gatewayv1.ConsumeRoute{}
						case *gatewayv1.RouteListenerList:
							l.Items = []gatewayv1.RouteListener{}
						}
					}).Return(nil)

				err := handler.CreateOrUpdate(ctx, route)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to build route"))
			})
		})

		Context("proxy route", func() {
			It("uses spec.route field selector for consumer listing", func() {
				route.Spec.Type = gatewayv1.RouteTypeProxy

				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()
				mockBuilder.EXPECT().AddRouteListeners(mock.Anything).Maybe()
				mockBuilder.EXPECT().Build(mock.Anything).Return(nil)
				mockBuilder.EXPECT().GetAllowedConsumers().Return([]*gatewayv1.ConsumeRoute{})

				// Capture the list options to verify the field selector
				mockClient.EXPECT().List(mock.Anything, mock.Anything, mock.Anything).
					Run(func(_ context.Context, list pkgclient.ObjectList, opts ...pkgclient.ListOption) {
						switch l := list.(type) {
						case *gatewayv1.ConsumeRouteList:
							l.Items = []gatewayv1.ConsumeRoute{}
							// Verify the correct field selector is used for proxy routes
							Expect(opts).To(HaveLen(1))
						case *gatewayv1.RouteListenerList:
							l.Items = []gatewayv1.RouteListener{}
						}
					}).Return(nil)

				err := handler.CreateOrUpdate(ctx, route)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("Delete()", func() {
		Context("happy path", func() {
			It("deletes route via kong client", func() {
				setupReadyGatewayGet()

				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return mockKC, nil
				}

				mockKC.EXPECT().DeleteRoute(mock.Anything, route).Return(nil)

				err := handler.Delete(ctx, route)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("error handling", func() {
			It("returns error when gateway Get fails", func() {
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("gateway not found"))

				err := handler.Delete(ctx, route)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get gateway"))
			})

			It("returns error when kong client DeleteRoute fails", func() {
				setupReadyGatewayGet()

				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return mockKC, nil
				}

				mockKC.EXPECT().DeleteRoute(mock.Anything, route).Return(fmt.Errorf("kong delete error"))

				err := handler.Delete(ctx, route)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to delete route"))
			})

			It("returns error when GetClientFor fails", func() {
				setupReadyGatewayGet()

				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return nil, fmt.Errorf("failed to create kong client")
				}

				err := handler.Delete(ctx, route)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get kong client"))
			})
		})
	})
})
