// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	approvalapi "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	cindex "github.com/telekom/controlplane/common/pkg/controller/index"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/labelutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subscription route readiness", Ordered, func() {
	var testEnv *envtest.Environment
	var scopedClient cclient.JanitorClient
	var k8sClient client.Client
	ctx := context.Background()

	BeforeAll(func() {
		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{
				filepath.Join("..", "..", "..", "..", "admin", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "..", "application", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "..", "approval", "config", "crd", "bases"),
				filepath.Join("..", "..", "..", "..", "gateway", "config", "crd", "bases"),
			},
			ErrorIfCRDPathMissing: true,
		}
		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(testEnv.Stop()).To(Succeed()) })
		scheme := runtime.NewScheme()
		Expect(adminapi.AddToScheme(scheme)).To(Succeed())
		Expect(apiapi.AddToScheme(scheme)).To(Succeed())
		Expect(applicationapi.AddToScheme(scheme)).To(Succeed())
		Expect(approvalapi.AddToScheme(scheme)).To(Succeed())
		Expect(gatewayapi.AddToScheme(scheme)).To(Succeed())
		reader, err := cache.New(cfg, cache.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(reader.IndexField(ctx, &apiapi.Api{}, "status.active", func(obj client.Object) []string {
			return []string{strconv.FormatBool(obj.(*apiapi.Api).Status.Active)}
		})).To(Succeed())
		Expect(reader.IndexField(ctx, &apiapi.ApiExposure{}, "status.active", func(obj client.Object) []string {
			return []string{strconv.FormatBool(obj.(*apiapi.ApiExposure).Status.Active)}
		})).To(Succeed())
		Expect(cindex.SetOwnerIndex(ctx, reader, &approvalapi.ApprovalRequest{})).To(Succeed())
		cacheCtx, cancel := context.WithCancel(ctx)
		stopped := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(stopped)
			Expect(reader.Start(cacheCtx)).To(Succeed())
		}()
		DeferCleanup(func() {
			cancel()
			Eventually(stopped, 5*time.Second, 10*time.Millisecond).Should(BeClosed())
		})
		Expect(reader.WaitForCacheSync(cacheCtx)).To(BeTrue())
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		cachedClient, err := client.New(cfg, client.Options{Scheme: scheme, Cache: &client.CacheOptions{Reader: reader}})
		Expect(err).NotTo(HaveOccurred())
		scopedClient = cclient.NewJanitorClient(cclient.NewScopedClient(cachedClient, "test"))
		zone := &adminapi.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name: "subscriber", Namespace: "default",
				Labels: map[string]string{config.EnvironmentLabelKey: "test"},
			},
			Spec: adminapi.ZoneSpec{
				Visibility: adminapi.ZoneVisibilityWorld,
				Gateways: []adminapi.GatewayConfig{{
					Name:  "gateway",
					Admin: adminapi.GatewayAdminConfig{IdentityProviderRef: "primary", Url: "http://gateway.example.com"},
				}},
				Presets: []adminapi.Preset{{
					Name: "default", Type: adminapi.GatewayTypeAPI, Default: true, GatewayRef: "gateway", IdentityProviderRef: "primary",
					Urls: []adminapi.UrlConfig{{Hostname: "gateway.example.com", Scheme: "https", Port: 443, BasePath: "/"}},
				}, {
					Name: "failover", Type: adminapi.GatewayTypeAPI, GatewayRef: "gateway", IdentityProviderRef: "primary",
					Features: []adminapi.Feature{{Name: adminapi.FeatureConsumerFailover, Enabled: true}},
					Urls:     []adminapi.UrlConfig{{Hostname: "failover.example.com", Scheme: "https", Port: 443, BasePath: "/"}},
				}},
				IdentityProviders: []adminapi.IdentityProviderConfig{{Name: "primary", IssuerHostname: "idp.example.com"}},
			},
		}
		Expect(k8sClient.Create(ctx, zone)).To(Succeed())
		zone.Status.Namespace = "subscriber-zone"
		zone.Status.Presets = []adminapi.PresetStatus{{Name: "default", Links: adminapi.Links{Url: "https://gateway.example.com", Issuer: "https://idp.example.com", LmsIssuer: "https://lms.example.com"}}}
		zone.Status.Presets = append(zone.Status.Presets, adminapi.PresetStatus{Name: "failover", Links: adminapi.Links{
			Url: "https://failover.example.com", Issuer: "https://failover-idp.example.com", LmsIssuer: "https://lms.example.com",
		}})
		zone.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Zone ready"))
		Expect(k8sClient.Status().Update(ctx, zone)).To(Succeed())
	})

	DescribeTable("publishes pending endpoint metadata without provisioning access",
		func(crossZone, failover bool) {
			name := "pending-" + strconv.FormatBool(crossZone) + "-" + strconv.FormatBool(failover)
			basePath := "/" + name + "/v1"
			metadata := func(resourceName string) metav1.ObjectMeta {
				return metav1.ObjectMeta{Name: resourceName, Namespace: "default", Labels: map[string]string{
					config.EnvironmentLabelKey: "test", apiapi.BasePathLabelKey: labelutil.NormalizeLabelValue(basePath),
					config.BuildLabelKey("application"): name,
				}}
			}
			app := &applicationapi.Application{ObjectMeta: metadata(name), Spec: applicationapi.ApplicationSpec{
				Team: name, TeamEmail: "team@example.com", Secret: "test-secret-reference",
			}}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			app.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Ready"))
			Expect(k8sClient.Status().Update(ctx, app)).To(Succeed())
			api := &apiapi.Api{ObjectMeta: metadata(name), Spec: apiapi.ApiSpec{BasePath: basePath}}
			Expect(k8sClient.Create(ctx, api)).To(Succeed())
			api.Status.Active = true
			Expect(k8sClient.Status().Update(ctx, api)).To(Succeed())
			exposure := &apiapi.ApiExposure{ObjectMeta: metadata(name), Spec: apiapi.ApiExposureSpec{
				ApiBasePath: basePath, Upstreams: []apiapi.Upstream{}, Visibility: apiapi.VisibilityWorld,
				Approval: apiapi.Approval{Strategy: apiapi.ApprovalStrategySimple},
				Zone:     types.ObjectRef{Name: "subscriber", Namespace: "default"},
			}}
			if crossZone {
				exposure.Spec.Zone.Name = "provider"
			}
			Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
			exposure.Status.Active = true
			exposure.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Ready"))
			Expect(k8sClient.Status().Update(ctx, exposure)).To(Succeed())
			sub := &apiapi.ApiSubscription{ObjectMeta: metadata(name), Spec: apiapi.ApiSubscriptionSpec{
				ApiBasePath: basePath, Zone: types.ObjectRef{Name: "subscriber", Namespace: "default"},
				Requestor: apiapi.Requestor{Application: *types.ObjectRefFromObject(app)},
			}}
			if failover {
				sub.Spec.Traffic.Failover = &apiapi.SubscriberFailover{Enabled: true}
			}
			Expect(k8sClient.Create(ctx, sub)).To(Succeed())
			testCtx := cclient.WithClient(ctx, scopedClient)
			handler := &ApiSubscriptionHandler{}
			expectedURL, expectedIssuer := "https://gateway.example.com:443"+basePath, "https://idp.example.com"
			if failover {
				expectedURL, expectedIssuer = "https://failover.example.com:443"+basePath, "https://failover-idp.example.com"
			}
			Eventually(func(g Gomega) {
				g.Expect(handler.CreateOrUpdate(testCtx, sub)).To(Succeed())
				g.Expect(sub.Status.GatewayUrl).To(Equal(expectedURL))
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
			Expect(sub.Status.IdpIssuer).To(Equal(expectedIssuer))
			Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Reason).To(Equal("ApprovalPending"))
			Expect(k8sClient.Status().Update(ctx, sub)).To(Succeed())
			persisted := &apiapi.ApiSubscription{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sub), persisted)).To(Succeed())
			Expect(persisted.Status.GatewayUrl).To(Equal(expectedURL))
			Expect(sub.Status.ApprovalRequest).NotTo(BeNil())
			request := &approvalapi.ApprovalRequest{}
			Expect(k8sClient.Get(ctx, sub.Status.ApprovalRequest.K8s(), request)).To(Succeed())
			Expect(request.Spec.State).To(Equal(approvalapi.ApprovalStatePending))
			sub.Status.GatewayUrl = ""
			Expect(handler.CreateOrUpdate(testCtx, sub)).To(Succeed())
			Expect(sub.Status.GatewayUrl).To(Equal(expectedURL))
			Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Reason).To(Equal("ApprovalPending"))
			routes := &gatewayapi.ConsumeRouteList{}
			Expect(k8sClient.List(ctx, routes)).To(Succeed())
			Expect(routes.Items).To(BeEmpty())

			approval := &approvalapi.Approval{
				ObjectMeta: metadata(approvalapi.ApprovalName("ApiSubscription", sub.Name)),
				Spec:       approvalapi.ApprovalSpec{State: approvalapi.ApprovalStateGranted, Strategy: approvalapi.ApprovalStrategySimple},
			}
			Expect(k8sClient.Create(ctx, approval)).To(Succeed())
			Eventually(func(g Gomega) {
				cached := &approvalapi.Approval{}
				g.Expect(scopedClient.Get(ctx, client.ObjectKeyFromObject(approval), cached)).To(Succeed())
				g.Expect(cached.Spec.State).To(Equal(approvalapi.ApprovalStateGranted))
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
			sub.Status.GatewayUrl = ""
			Expect(handler.CreateOrUpdate(testCtx, sub)).To(Succeed())
			Expect(sub.Status.GatewayUrl).To(Equal(expectedURL))
			Expect(meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady).Reason).To(Equal(condition.ReasonPreconditionNotMet))
		},
		Entry("same zone", false, false),
		Entry("cross zone", true, false),
		Entry("consumer failover", true, true),
	)

	It("returns an error when the subscription zone is missing", func() {
		sub := &apiapi.ApiSubscription{Spec: apiapi.ApiSubscriptionSpec{Zone: types.ObjectRef{Name: "missing", Namespace: "default"}}}
		Expect(setGatewayURL(ctx, scopedClient, sub)).To(MatchError(ContainSubstring("failed to get subscription zone")))
		Expect(sub.Status.GatewayUrl).To(BeEmpty())
	})

	DescribeTable("preserves gateway preset resolution rules",
		func(name string, configure func(*adminapi.Zone), errorMessage string) {
			zone := &adminapi.Zone{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "subscriber", Namespace: "default"}, zone)).To(Succeed())
			zone.ObjectMeta = metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{
				config.EnvironmentLabelKey: "test",
			}}
			configure(zone)
			status := zone.Status
			Expect(k8sClient.Create(ctx, zone)).To(Succeed())
			zone.Status = status
			Expect(k8sClient.Status().Update(ctx, zone)).To(Succeed())
			Eventually(func(g Gomega) {
				cached := &adminapi.Zone{}
				g.Expect(scopedClient.Get(ctx, client.ObjectKeyFromObject(zone), cached)).To(Succeed())
				g.Expect(cached.ResourceVersion).To(Equal(zone.ResourceVersion))
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
			sub := &apiapi.ApiSubscription{Spec: apiapi.ApiSubscriptionSpec{
				ApiBasePath: "/checkout/v1", Zone: *types.ObjectRefFromObject(zone),
				Traffic: apiapi.SubscriberTraffic{Failover: &apiapi.SubscriberFailover{Enabled: true}},
			}}
			err := setGatewayURL(ctx, scopedClient, sub)
			if errorMessage != "" {
				Expect(err).To(MatchError(ContainSubstring(errorMessage)))
				Expect(sub.Status.GatewayUrl).To(BeEmpty())
				Expect(sub.Status.IdpIssuer).To(BeEmpty())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(sub.Status.GatewayUrl).To(Equal("https://gateway.example.com:443/checkout/v1"))
			Expect(sub.Status.IdpIssuer).To(Equal("https://idp.example.com"))
		},
		Entry("falls back only when no failover preset exists", "no-failover", func(zone *adminapi.Zone) {
			zone.Spec.Presets = zone.Spec.Presets[:1]
		}, ""),
		Entry("rejects ambiguous failover defaults", "ambiguous-failover", func(zone *adminapi.Zone) {
			for i := range zone.Spec.Presets {
				zone.Spec.Presets[i].Default = true
				zone.Spec.Presets[i].Features = []adminapi.Feature{{Name: adminapi.FeatureConsumerFailover, Enabled: true}}
			}
		}, "failed to select failover preset"),
		Entry("requires the selected preset status", "missing-preset-status", func(zone *adminapi.Zone) {
			zone.Status.Presets = nil
		}, "failed to get preset status"),
	)

	DescribeTable("clears previous readiness when the required route reference is missing",
		func(sameZone, failover bool, message string) {
			sub := &apiapi.ApiSubscription{Spec: apiapi.ApiSubscriptionSpec{
				Zone: types.ObjectRef{Name: "subscriber", Namespace: "default"},
			}}
			sub.SetCondition(condition.NewReadyCondition(condition.ReasonProvisioned, "Previously ready"))
			ref, err := resolveRouteRef(ctx, scopedClient, sub, &apiapi.ApiExposure{}, sameZone, failover)
			Expect(err).NotTo(HaveOccurred())
			Expect(ref).To(BeNil())
			ready := meta.FindStatusCondition(sub.GetConditions(), condition.ConditionTypeReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(condition.ReasonPreconditionNotMet))
			Expect(ready.Message).To(Equal(message))
		},
		Entry("primary route", true, false, "Waiting for ApiExposure to create the route"),
		Entry("provider failover route", false, true, "Waiting for ApiExposure to create the failover route for this zone"),
		Entry("proxy route", false, false, "Waiting for ApiExposure to create the proxy route for this zone"),
	)
})
