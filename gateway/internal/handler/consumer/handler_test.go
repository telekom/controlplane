// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package consumer_test

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
	consumerhandler "github.com/telekom/controlplane/gateway/internal/handler/consumer"
	kongclient "github.com/telekom/controlplane/gateway/pkg/kong/client"
	clientmock "github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"
	secrets "github.com/telekom/controlplane/secret-manager/api"
)

var _ = Describe("ConsumerHandler", func() {
	var (
		ctx         context.Context
		handler     *consumerhandler.ConsumerHandler
		mockClient  *fakeclient.MockJanitorClient
		mockKC      *clientmock.MockKongClient
		mockBuilder *featmock.MockFeaturesBuilder
		consumer    *gatewayv1.Consumer
	)

	BeforeEach(func() {
		handler = &consumerhandler.ConsumerHandler{}
		mockClient = fakeclient.NewMockJanitorClient(GinkgoT())
		mockKC = clientmock.NewMockKongClient(GinkgoT())
		mockBuilder = featmock.NewMockFeaturesBuilder(GinkgoT())

		ctx = cc.WithClient(context.Background(), mockClient)

		consumer = &gatewayv1.Consumer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-consumer",
				Namespace: "test-ns",
			},
			Spec: gatewayv1.ConsumerSpec{
				Gateway: types.ObjectRef{
					Name:      "test-gateway",
					Namespace: "test-ns",
				},
				Name: "my-consumer",
			},
		}

		// Override secrets.Get to be a no-op (return the value as-is)
		originalSecretsGet := secrets.Get
		DeferCleanup(func() { secrets.Get = originalSecretsGet })
		secrets.Get = func(_ context.Context, ref string) (string, error) {
			return ref, nil
		}
	})

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
			It("builds for consumer and sets Ready condition", func() {
				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()
				mockBuilder.EXPECT().BuildForConsumer(mock.Anything).Return(nil)

				err := handler.CreateOrUpdate(ctx, consumer)
				Expect(err).NotTo(HaveOccurred())

				Expect(meta.IsStatusConditionTrue(consumer.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				Expect(meta.IsStatusConditionFalse(consumer.GetConditions(), condition.ConditionTypeProcessing)).To(BeTrue())
			})
		})

		Context("error handling", func() {
			It("returns error when feature builder creation fails (gateway not ready)", func() {
				setupNotReadyGatewayGet()

				// Don't override features.NewFeatureBuilder - let the real one see the not-ready gateway
				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return mockKC, nil
				}

				err := handler.CreateOrUpdate(ctx, consumer)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create feature builder"))
			})

			It("returns error when BuildForConsumer fails", func() {
				setupReadyGatewayGet()
				setupFeatureBuilderOverrides()

				mockBuilder.EXPECT().EnableFeature(mock.Anything).Maybe()
				mockBuilder.EXPECT().BuildForConsumer(mock.Anything).Return(fmt.Errorf("build consumer failed"))

				err := handler.CreateOrUpdate(ctx, consumer)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to build consumer"))
			})

			It("returns error when gateway Get fails", func() {
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("gateway not found"))

				err := handler.CreateOrUpdate(ctx, consumer)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create feature builder"))
			})
		})
	})

	Describe("Delete()", func() {
		Context("happy path", func() {
			It("deletes consumer via kong client", func() {
				setupReadyGatewayGet()

				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return mockKC, nil
				}

				mockKC.EXPECT().DeleteConsumer(mock.Anything, consumer).Return(nil)

				err := handler.Delete(ctx, consumer)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("error handling", func() {
			It("returns error when gateway Get fails", func() {
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("gateway not found"))

				err := handler.Delete(ctx, consumer)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get gateway"))
			})

			It("returns error when DeleteConsumer fails", func() {
				setupReadyGatewayGet()

				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return mockKC, nil
				}

				mockKC.EXPECT().DeleteConsumer(mock.Anything, consumer).Return(fmt.Errorf("kong delete error"))

				err := handler.Delete(ctx, consumer)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create or update consumer"))
			})

			It("returns error when GetClientFor fails", func() {
				setupReadyGatewayGet()

				originalGetClientFor := kongutil.GetClientFor
				DeferCleanup(func() { kongutil.GetClientFor = originalGetClientFor })
				kongutil.GetClientFor = func(_ kongutil.GatewayAdminConfig) (kongclient.KongClient, error) {
					return nil, fmt.Errorf("failed to create kong client")
				}

				err := handler.Delete(ctx, consumer)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get kong client"))
			})
		})
	})
})
