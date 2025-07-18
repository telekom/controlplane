// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"context"

	"github.com/emirpasic/gods/sets/hashset"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/internal/features/feature"
	featuresmock "github.com/telekom/controlplane/gateway/internal/features/mock"
	kong "github.com/telekom/controlplane/gateway/pkg/kong/api"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/plugin"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewConsumer() *gatewayv1.Consumer {
	return &gatewayv1.Consumer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-consumer",
			Namespace: "test-namespace",
		},
		Spec: gatewayv1.ConsumerSpec{
			Name: "test-consumer",
		},
	}
}

// Helper function to create a string pointer for Kong API types
func stringPtr(s string) *string {
	return &s
}

var _ = Describe("IpRestrictionFeature", func() {
	It("should have the correct priority", func() {
		Expect(feature.InstanceIpRestrictionFeature.Priority()).To(Equal(10))
	})

	It("should return the correct feature type", func() {
		Expect(feature.InstanceIpRestrictionFeature.Name()).To(Equal(gatewayv1.FeatureTypeIpRestriction))
	})

	Context("with mocked feature builder", func() {
		var ctrl *gomock.Controller
		var mockFeatureBuilder *featuresmock.MockFeaturesBuilder
		var ipRestrictionFeature feature.IpRestrictionFeature

		BeforeEach(func() {
			ipRestrictionFeature = feature.IpRestrictionFeature{}
			ctrl = gomock.NewController(GinkgoT())
			mockFeatureBuilder = featuresmock.NewMockFeaturesBuilder(ctrl)
		})

		Context("check IsUsed", func() {
			It("consumer with no IP restriction config, feature should not be used", func() {
				consumer := NewConsumer()
				mockFeatureBuilder.EXPECT().GetConsumer().Return(consumer, true)
				Expect(ipRestrictionFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})

			It("consumer with IP restriction config, feature should be used", func() {
				consumer := NewConsumer()
				consumer.Spec.Security = &gatewayv1.ConsumerSecurity{
					IpRestrictions: &gatewayv1.IpRestrictions{
						Allow: []string{"192.168.1.1"},
					},
				}
				mockFeatureBuilder.EXPECT().GetConsumer().Return(consumer, true)
				Expect(ipRestrictionFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeTrue())
			})

			It("no consumer available, feature should not be used", func() {
				mockFeatureBuilder.EXPECT().GetConsumer().Return(nil, false)
				Expect(ipRestrictionFeature.IsUsed(context.Background(), mockFeatureBuilder)).To(BeFalse())
			})
		})

		Context("Apply", func() {
			It("should return error when no consumer available", func() {
				mockFeatureBuilder.EXPECT().GetConsumer().Return(nil, false)
				err := ipRestrictionFeature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).To(Equal(features.ErrNoConsumer))
			})

			It("should configure IP restriction plugin with allow list", func() {
				consumer := NewConsumer()
				consumer.Spec.Security = &gatewayv1.ConsumerSecurity{
					IpRestrictions: &gatewayv1.IpRestrictions{
						Allow: []string{"192.168.1.1", "10.0.0.0/24"},
					},
				}

				mockIpPlugin := &plugin.IpRestrictionPlugin{
					Config: plugin.IpRestrictionPluginConfig{
						Allow: hashset.New(),
						Deny:  hashset.New(),
					},
					Id: "",
				}

				mockFeatureBuilder.EXPECT().GetConsumer().Return(consumer, true)
				mockFeatureBuilder.EXPECT().IpRestrictionPlugin().Return(mockIpPlugin)

				err := ipRestrictionFeature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).ToNot(HaveOccurred())
				Expect(mockIpPlugin.Config.Allow.Contains("192.168.1.1")).To(BeTrue())
				Expect(mockIpPlugin.Config.Allow.Contains("10.0.0.0/24")).To(BeTrue())
			})

			It("should configure IP restriction plugin with deny list", func() {
				consumer := NewConsumer()
				consumer.Spec.Security = &gatewayv1.ConsumerSecurity{
					IpRestrictions: &gatewayv1.IpRestrictions{
						Deny: []string{"192.168.1.2", "10.0.1.0/24"},
					},
				}

				mockIpPlugin := &plugin.IpRestrictionPlugin{
					Config: plugin.IpRestrictionPluginConfig{
						Allow: hashset.New(),
						Deny:  hashset.New(),
					},
					Id: "",
				}

				mockFeatureBuilder.EXPECT().GetConsumer().Return(consumer, true)
				mockFeatureBuilder.EXPECT().IpRestrictionPlugin().Return(mockIpPlugin)

				err := ipRestrictionFeature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).ToNot(HaveOccurred())
				Expect(mockIpPlugin.Config.Deny.Contains("192.168.1.2")).To(BeTrue())
				Expect(mockIpPlugin.Config.Deny.Contains("10.0.1.0/24")).To(BeTrue())
			})

			It("should configure IP restriction plugin with both allow and deny lists", func() {
				consumer := NewConsumer()
				consumer.Spec.Security = &gatewayv1.ConsumerSecurity{
					IpRestrictions: &gatewayv1.IpRestrictions{
						Allow: []string{"192.168.1.1", "10.0.0.0/24"},
						Deny:  []string{"192.168.1.2", "10.0.1.0/24"},
					},
				}

				mockIpPlugin := &plugin.IpRestrictionPlugin{
					Config: plugin.IpRestrictionPluginConfig{
						Allow: hashset.New(),
						Deny:  hashset.New(),
					},
					Id: "",
				}

				mockFeatureBuilder.EXPECT().GetConsumer().Return(consumer, true)
				mockFeatureBuilder.EXPECT().IpRestrictionPlugin().Return(mockIpPlugin)

				err := ipRestrictionFeature.Apply(context.Background(), mockFeatureBuilder)
				Expect(err).ToNot(HaveOccurred())
				Expect(mockIpPlugin.Config.Allow.Contains("192.168.1.1")).To(BeTrue())
				Expect(mockIpPlugin.Config.Allow.Contains("10.0.0.0/24")).To(BeTrue())
				Expect(mockIpPlugin.Config.Deny.Contains("192.168.1.2")).To(BeTrue())
				Expect(mockIpPlugin.Config.Deny.Contains("10.0.1.0/24")).To(BeTrue())
			})
		})
	})

	Context("correctly configure IP restriction", func() {
		var ctx context.Context
		var mockCtrl *gomock.Controller

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			ctx = context.Background()
		})

		It("should apply IP restriction feature for a consumer with allow list", func() {
			mockKc := mock.NewMockKongClient(mockCtrl)

			consumer := NewConsumer()
			consumer.Spec.Security = &gatewayv1.ConsumerSecurity{
				IpRestrictions: &gatewayv1.IpRestrictions{
					Allow: []string{"192.168.1.1", "10.0.0.0/24"},
				},
			}

			// Mock CreateOrReplaceConsumer which will be called by the builder
			mockKc.EXPECT().CreateOrReplaceConsumer(ctx, gomock.Any()).Return(&kong.Consumer{Id: stringPtr("test-consumer-id")}, nil).Times(1)

			// Mock CreateOrReplacePlugin for the IP restriction plugin
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).DoAndReturn(
				func(ctx context.Context, customPlugin client.CustomPlugin) (*kong.Plugin, error) {
					ipPlugin, ok := customPlugin.(*plugin.IpRestrictionPlugin)
					Expect(ok).To(BeTrue())
					config := ipPlugin.GetConfig()
					allowSet, ok := config["allow"].(*hashset.Set)
					Expect(ok).To(BeTrue())
					Expect(allowSet.Contains("192.168.1.1")).To(BeTrue())
					Expect(allowSet.Contains("10.0.0.0/24")).To(BeTrue())
					return &kong.Plugin{Id: stringPtr("test-plugin-id")}, nil
				}).Times(1)

			// Mock CleanupPlugins which will be called by the builder
			mockKc.EXPECT().CleanupPlugins(ctx, nil, gomock.Any(), gomock.Any()).Return(nil).Times(1)

			builder := features.NewFeatureBuilder(mockKc, nil, consumer, nil, nil)
			builder.EnableFeature(feature.InstanceIpRestrictionFeature)

			err := builder.BuildForConsumer(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should apply IP restriction feature for a consumer with deny list", func() {
			mockKc := mock.NewMockKongClient(mockCtrl)

			consumer := NewConsumer()
			consumer.Spec.Security = &gatewayv1.ConsumerSecurity{
				IpRestrictions: &gatewayv1.IpRestrictions{
					Deny: []string{"192.168.1.2", "10.0.1.0/24"},
				},
			}

			// Mock CreateOrReplaceConsumer which will be called by the builder
			mockKc.EXPECT().CreateOrReplaceConsumer(ctx, gomock.Any()).Return(&kong.Consumer{Id: stringPtr("test-consumer-id")}, nil).Times(1)

			// Mock CreateOrReplacePlugin for the IP restriction plugin
			mockKc.EXPECT().CreateOrReplacePlugin(ctx, gomock.Any()).DoAndReturn(
				func(ctx context.Context, customPlugin client.CustomPlugin) (*kong.Plugin, error) {
					ipPlugin, ok := customPlugin.(*plugin.IpRestrictionPlugin)
					Expect(ok).To(BeTrue())
					config := ipPlugin.GetConfig()
					denySet, ok := config["deny"].(*hashset.Set)
					Expect(ok).To(BeTrue())
					Expect(denySet.Contains("192.168.1.2")).To(BeTrue())
					Expect(denySet.Contains("10.0.1.0/24")).To(BeTrue())
					return &kong.Plugin{Id: stringPtr("test-plugin-id")}, nil
				}).Times(1)

			// Mock CleanupPlugins which will be called by the builder
			mockKc.EXPECT().CleanupPlugins(ctx, nil, gomock.Any(), gomock.Any()).Return(nil).Times(1)

			builder := features.NewFeatureBuilder(mockKc, nil, consumer, nil, nil)
			builder.EnableFeature(feature.InstanceIpRestrictionFeature)

			err := builder.BuildForConsumer(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
