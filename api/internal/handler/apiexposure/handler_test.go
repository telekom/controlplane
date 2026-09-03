// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure

import (
	"context"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	organizationapi "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ──────────────────────────────────────────────────────────────────────────────
// Test fixture builders
// ──────────────────────────────────────────────────────────────────────────────

const (
	testEnv      = "test-env"
	testBasePath = "/my/api/v1"
	realmA       = "realm-a"
	realmB       = "realm-b"
	realmC       = "realm-c"
	realmF       = "realm-f"
)

func makeReadyApi() apiapi.Api {
	basePath := testBasePath
	api := apiapi.Api{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-api",
			Namespace: "test-env--grp--team",
			Labels:    map[string]string{apiapi.BasePathLabelKey: basePath},
		},
		Spec: apiapi.ApiSpec{
			BasePath: basePath,
		},
		Status: apiapi.ApiStatus{
			Active: true,
		},
	}
	meta.SetStatusCondition(&api.Status.Conditions, metav1.Condition{
		Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
	})
	return api
}

func makeReadyZone(name, namespace, realmName, issuer, lmsIssuer string, presets ...adminv1.Preset) *adminv1.Zone {
	if len(presets) == 0 {
		presets = []adminv1.Preset{{
			Name: "default", Default: true,
			Urls: []adminv1.UrlConfig{{
				Hostname: name + ".gw.example.com",
				Scheme:   "https",
				BasePath: "/",
			}},
		}}
	}
	for i := range presets {
		presets[i].GatewayRef = "standard"
	}
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: adminv1.ZoneSpec{
			Gateways: []adminv1.GatewayConfig{{Name: "standard", Types: []adminv1.GatewayType{adminv1.GatewayTypeAPI}}},
			Presets:  presets,
		},
		Status: adminv1.ZoneStatus{
			Namespace: namespace,
			RealmName: realmName,
			Presets: []adminv1.PresetStatus{{Name: presets[0].Name, GatewayRef: &ctypes.ObjectRef{Name: "gw-" + name, Namespace: namespace}, Links: adminv1.Links{
				Issuer:    issuer,
				LmsIssuer: lmsIssuer,
			}}},
		},
	}
	for _, preset := range presets[1:] {
		z.Status.Presets = append(z.Status.Presets, adminv1.PresetStatus{
			Name: preset.Name, GatewayRef: &ctypes.ObjectRef{Name: "gw-" + name, Namespace: namespace},
			Links: adminv1.Links{Issuer: issuer, LmsIssuer: lmsIssuer},
		})
	}
	meta.SetStatusCondition(&z.Status.Conditions, metav1.Condition{
		Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
	})
	return z
}

func enableConsumerFailover(z *adminv1.Zone) *adminv1.Zone {
	z.EnableFeature(adminv1.FeatureConsumerFailover)
	return z
}

func newApiExposure(zone ctypes.ObjectRef) *apiapi.ApiExposure {
	basePath := testBasePath
	return &apiapi.ApiExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exposure",
			Namespace: "test-env--grp--team",
			UID:       "exp-uid",
			Labels: map[string]string{
				apiapi.BasePathLabelKey:  basePath,
				util.ApplicationLabelKey: "test-app",
			},
		},
		Spec: apiapi.ApiExposureSpec{
			ApiBasePath: basePath,
			Zone:        zone,
			Upstreams:   []apiapi.Upstream{{Url: "https://backend.internal:8080" + basePath, Weight: 100}},
			Visibility:  apiapi.VisibilityWorld,
			Approval:    apiapi.Approval{Strategy: apiapi.ApprovalStrategyAuto},
		},
	}
}

func makeSubscription(basePath string, zone ctypes.ObjectRef, failover bool) apiapi.ApiSubscription {
	sub := apiapi.ApiSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sub-" + zone.Name,
			Namespace: "test-env--grp--team",
			Labels:    map[string]string{apiapi.BasePathLabelKey: basePath},
		},
		Spec: apiapi.ApiSubscriptionSpec{
			ApiBasePath: basePath,
			Zone:        zone,
		},
	}
	if failover {
		sub.Spec.Traffic = apiapi.SubscriberTraffic{
			Failover: &apiapi.SubscriberFailover{Enabled: true},
		}
	}
	return sub
}

// ──────────────────────────────────────────────────────────────────────────────
// Tests
// ──────────────────────────────────────────────────────────────────────────────

var _ = Describe("ApiExposureHandler", func() {
	const basePath = "/my/api/v1"

	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *ApiExposureHandler
	)

	// Zone definitions reused across scenarios
	var (
		zoneARef = ctypes.ObjectRef{Name: "zone-a", Namespace: "ns-a"}
		zoneBRef = ctypes.ObjectRef{Name: "zone-b", Namespace: "ns-b"}
		zoneCRef = ctypes.ObjectRef{Name: "zone-c", Namespace: "ns-c"}
		zoneFRef = ctypes.ObjectRef{Name: "zone-f", Namespace: "ns-f"}
	)

	var (
		zoneA = func() *adminv1.Zone {
			return makeReadyZone("zone-a", "ns-a", realmA,
				"https://idp.zone-a.example.com", "https://lms.zone-a.example.com")
		}
		zoneB = func() *adminv1.Zone {
			return makeReadyZone("zone-b", "ns-b", realmB,
				"https://idp.zone-b.example.com", "https://lms.zone-b.example.com")
		}
		zoneF = func() *adminv1.Zone {
			return makeReadyZone("zone-f", "ns-f", realmF,
				"https://idp.zone-f.example.com", "https://lms.zone-f.example.com")
		}
	)

	// Consumer-failover-enabled zone constructors
	var (
		zoneACF = func() *adminv1.Zone {
			z := makeReadyZone("zone-a", "ns-a", realmA,
				"https://idp.zone-a.example.com", "https://lms.zone-a.example.com",
				adminv1.Preset{
					Name: "default", Default: true,
					Urls: []adminv1.UrlConfig{{Hostname: "zone-a.gw.example.com", Scheme: "https", BasePath: "/"}},
				},
				adminv1.Preset{
					Name:     "consumer-failover",
					Urls:     []adminv1.UrlConfig{{Hostname: "zone-a.cf.example.com", Scheme: "https", BasePath: "/cf"}},
					Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}},
				},
			)
			return enableConsumerFailover(z)
		}
		zoneCCF = func() *adminv1.Zone {
			z := makeReadyZone("zone-c", "ns-c", realmC,
				"https://idp.zone-c.example.com", "https://lms.zone-c.example.com",
				adminv1.Preset{
					Name: "default", Default: true,
					Urls: []adminv1.UrlConfig{{Hostname: "zone-c.gw.example.com", Scheme: "https", BasePath: "/"}},
				},
				adminv1.Preset{
					Name:     "consumer-failover",
					Urls:     []adminv1.UrlConfig{{Hostname: "zone-c.cf.example.com", Scheme: "https", BasePath: "/cf"}},
					Features: []adminv1.Feature{{Name: adminv1.FeatureConsumerFailover, Enabled: true}},
				},
			)
			return enableConsumerFailover(z)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, testEnv)
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &ApiExposureHandler{}
	})

	// ──────────────────────────────────────────────────────────────────────
	// Mock helpers
	// ──────────────────────────────────────────────────────────────────────

	// mockListApis mocks the List call for FindActiveAPI.
	mockListApis := func(items []apiapi.Api) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiList"), mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*apiapi.ApiList) = apiapi.ApiList{Items: items}
			}).
			Return(nil).Once()
	}

	// mockListApiExposures mocks the List call for FindActiveAPIExposure.
	mockListApiExposures := func(items []apiapi.ApiExposure) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiExposureList"), mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*apiapi.ApiExposureList) = apiapi.ApiExposureList{Items: items}
			}).
			Return(nil).Once()
	}

	// mockListSubscriptions mocks the List call for FindAllSubscribersForApiExposure.
	mockListSubscriptions := func(items []apiapi.ApiSubscription) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiSubscriptionList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*apiapi.ApiSubscriptionList) = apiapi.ApiSubscriptionList{Items: items}
			}).
			Return(nil).Once()
	}

	// mockGetZone mocks a Get call for a specific Zone by key.
	mockGetZone := func(key k8stypes.NamespacedName, zone *adminv1.Zone, times int) {
		fakeClient.EXPECT().
			Get(ctx, key, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil).Times(times)
	}

	// mockListZones mocks the List call for FindAllZonesWithFeatureEnabled.
	mockListZones := func(zones []*adminv1.Zone) {
		items := make([]adminv1.Zone, len(zones))
		for i, z := range zones {
			items[i] = *z
		}
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ZoneList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*adminv1.ZoneList) = adminv1.ZoneList{Items: items}
			}).
			Return(nil).Once()
	}

	// capturedRoutes collects Route objects created by CreateOrUpdate calls.
	// Returns the slice pointer — append happens inside the mock's Run callback.
	capturedRoutes := func(routes *[]*gatewayapi.Route, times int) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				Expect(mutate()).To(Succeed())
				r := obj.(*gatewayapi.Route)
				cp := r.DeepCopy()
				*routes = append(*routes, cp)
			}).
			Return(controllerutil.OperationResultCreated, nil).Times(times)
	}

	// mockCleanup mocks the Cleanup call for CleanupStaleRoutes.
	mockCleanup := func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(0, nil).Once()
	}

	// mockGetApplication mocks the Get call for GetApplicationFromLabel.
	// Returns a ready Application so the handler proceeds past the application check.
	mockGetApplication := func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "test-app", Namespace: "test-env--grp--team"}, mock.AnythingOfType("*v1.Application")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				app := out.(*applicationapi.Application)
				app.Name = "test-app"
				app.Namespace = "test-env--grp--team"
				meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
					Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
				})
			}).
			Return(nil).Once()
	}

	// mockGetTeamNotFound mocks the Get call for FindTeamForObject, returning NotFound
	// so that validateApiCategoryPolicy skips validation and returns true.
	mockGetTeamNotFound := func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "grp--team", Namespace: "test-env"}, mock.AnythingOfType("*v1.Team")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "organization", Resource: "teams"}, "grp--team")).Once()
	}

	// ──────────────────────────────────────────────────────────────────────
	// Scenarios
	// ──────────────────────────────────────────────────────────────────────

	Describe("CreateOrUpdate", func() {
		// Scenario 1: Baseline — one cross-zone subscriber, no failover
		//
		// Exposure in zone-a, subscriber in zone-b, no consumer or provider failover.
		// Expects: 1 proxy route (zone-b) + 1 real route (zone-a).
		Context("baseline: one cross-zone subscriber, no failover", func() {
			It("creates a proxy route in the subscriber zone and a real route in the exposure zone", func() {
				obj := newApiExposure(zoneARef)

				// --- Preamble: Api + ApiExposure validation ---
				mockListApis([]apiapi.Api{makeReadyApi()})
				mockListApiExposures([]apiapi.ApiExposure{*obj}) // self → active=true
				mockGetApplication()
				mockGetTeamNotFound()

				// --- Step 1: determineRoutingState ---
				mockListSubscriptions([]apiapi.ApiSubscription{
					makeSubscription(basePath, zoneBRef, false),
				})

				// --- Step 2: manageProxyRoutes ---
				// Zone Gets: determineRoutingState(A) + LMS-collection(B) +
				//            downstream-proxy(B) + upstream-proxy(A) + real-route(A)
				mockGetZone(zoneARef.K8s(), zoneA(), 3) // determineRoutingState + upstream + real route
				mockGetZone(zoneBRef.K8s(), zoneB(), 2) // LMS collection + downstream

				var routes []*gatewayapi.Route
				capturedRoutes(&routes, 2) // 1 proxy + 1 real

				mockCleanup()

				// --- Execute ---
				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// --- Assertions ---
				Expect(routes).To(HaveLen(2))

				// Proxy route (created first)
				proxy := routes[0]
				Expect(proxy.Labels).To(HaveKeyWithValue("cp.ei.telekom.de/type", "proxy"))
				// Proxy route is owner-stamped (informational: records the provisioning owner)
				Expect(proxy.Labels).To(HaveKeyWithValue("cp.ei.telekom.de/owner.uid", "exp-uid"))
				Expect(proxy.Namespace).To(Equal("ns-b"))
				Expect(proxy.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
				Expect(proxy.Spec.Hostnames).To(ContainElement("zone-b.gw.example.com"))
				// Proxy route trusts the downstream (subscriber) zone's IDP issuer
				Expect(proxy.Spec.Security.TrustedIssuers).To(ConsistOf("https://idp.zone-b.example.com"))
				Expect(proxy.Spec.Security.RealmName).To(Equal(realmB))

				// Real route (created second)
				real := routes[1]
				Expect(real.Labels).To(HaveKeyWithValue("cp.ei.telekom.de/type", "real"))
				// Real route is owner-stamped as well
				Expect(real.Labels).To(HaveKeyWithValue("cp.ei.telekom.de/owner.uid", "exp-uid"))
				Expect(real.Namespace).To(Equal("ns-a"))
				Expect(real.Spec.Type).To(Equal(gatewayapi.RouteTypePrimary))
				Expect(real.Spec.Hostnames).To(ContainElement("zone-a.gw.example.com"))
				// Cross-zone subscriber → DefaultConsumers includes gateway
				Expect(real.Spec.Security.DefaultConsumers).To(ContainElement("gateway"))
				Expect(real.Spec.Security.RealmName).To(Equal(realmA))
				// Real route trusts LMS issuers from all cross-zone proxy zones
				Expect(real.Spec.Security.TrustedIssuers).To(ConsistOf("https://lms.zone-b.example.com"))

				// Status
				Expect(obj.Status.Route).ToNot(BeNil())
				Expect(obj.Status.ProxyRoutes).To(HaveLen(1))
				readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		// Scenario 2: Consumer failover
		//
		// Exposure in zone-a, subscriber in zone-b with HasFailover()=true.
		// Zones A and C have ConsumerFailover feature enabled with dedicated presets; zone-b does not.
		// Expects: 2 proxy routes (zone-b + zone-c) + 1 real route. Only CF-capable zones (A, C) are
		// enriched with failover hostnames/issuers; the plain zone-b proxy route is not.
		Context("consumer failover: enriches only CF-capable zones with failover hostnames and issuers", func() {
			It("enriches the CF-capable proxy route and the real route, but not the plain subscriber zone", func() {
				obj := newApiExposure(zoneARef)

				// --- Preamble ---
				mockListApis([]apiapi.Api{makeReadyApi()})
				mockListApiExposures([]apiapi.ApiExposure{*obj})
				mockGetApplication()
				mockGetTeamNotFound()

				// --- Step 1: determineRoutingState ---
				mockListSubscriptions([]apiapi.ApiSubscription{
					makeSubscription(basePath, zoneBRef, true), // consumer failover enabled
				})

				// --- Step 2: manageProxyRoutes ---
				// Consumer failover → list all zones
				mockListZones([]*adminv1.Zone{zoneACF(), zoneCCF()})

				// Zone Gets: determineRoutingState(A) + LMS-collection(B,C) +
				//            downstream-proxy-B(B) + upstream-proxy-B(A) +
				//            downstream-proxy-C(C) + upstream-proxy-C(A) + real-route(A)
				mockGetZone(zoneARef.K8s(), zoneACF(), 4) // determineRoutingState + upstream×2 + real route
				mockGetZone(zoneBRef.K8s(), zoneB(), 2)   // LMS collection + downstream proxy-B (plain zone, no CF feature)
				mockGetZone(zoneCRef.K8s(), zoneCCF(), 2) // LMS collection + downstream proxy-C (CF-enabled zone)

				var routes []*gatewayapi.Route
				capturedRoutes(&routes, 3) // 2 proxy + 1 real

				mockCleanup()

				// --- Execute ---
				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// --- Assertions ---
				Expect(routes).To(HaveLen(3))

				// Proxy route for zone-b (index 0)
				proxyB := routes[0]
				Expect(proxyB.Namespace).To(Equal("ns-b"))
				Expect(proxyB.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
				// zone-b lacks the ConsumerFailover feature, so its proxy route is NOT enriched
				// with failover hostnames — it cannot serve hostnames it has no gateway preset for.
				Expect(proxyB.Spec.Hostnames).To(ContainElement("zone-b.gw.example.com"))
				Expect(proxyB.Spec.Hostnames).ToNot(ContainElements("zone-a.cf.example.com", "zone-c.cf.example.com"))
				// Proxy route trusts only its own downstream IDP issuer
				Expect(proxyB.Spec.Security.TrustedIssuers).To(ConsistOf(
					"https://idp.zone-b.example.com", // downstream zone's own issuer
				))
				Expect(proxyB.Spec.Security.RealmName).To(Equal(realmB))

				// Proxy route for zone-c (index 1)
				proxyC := routes[1]
				Expect(proxyC.Namespace).To(Equal("ns-c"))
				Expect(proxyC.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
				Expect(proxyC.Spec.Hostnames).To(ContainElements("zone-a.cf.example.com", "zone-c.cf.example.com"))
				Expect(proxyC.Spec.Security.TrustedIssuers).To(ConsistOf(
					"https://idp.zone-a.example.com", // consumer failover from zone-a
					"https://idp.zone-c.example.com", // downstream + consumer failover (deduped)
				))
				Expect(proxyC.Spec.Security.RealmName).To(Equal(realmC))

				// Real route (index 2)
				real := routes[2]
				Expect(real.Namespace).To(Equal("ns-a"))
				Expect(real.Spec.Type).To(Equal(gatewayapi.RouteTypePrimary))
				// Real route also gets consumer failover hostnames
				Expect(real.Spec.Hostnames).To(ContainElements("zone-a.cf.example.com", "zone-c.cf.example.com"))
				// Real route TrustedIssuers: LMS from B+C + consumer failover IDP issuers from A+C
				Expect(real.Spec.Security.TrustedIssuers).To(ContainElements(
					"https://lms.zone-b.example.com", // mesh trust (LMS from zone-b)
					"https://lms.zone-c.example.com", // mesh trust (LMS from zone-c)
					"https://idp.zone-a.example.com", // consumer failover (IDP)
					"https://idp.zone-c.example.com", // consumer failover (IDP)
				))
				// Consumer failover paths
				Expect(real.Spec.Paths).To(ContainElements(
					ContainSubstring("/cf"),
				))
				// Cross-zone subscriber → gateway in DefaultConsumers
				Expect(real.Spec.Security.DefaultConsumers).To(ContainElement("gateway"))
				Expect(real.Spec.Security.RealmName).To(Equal(realmA))

				// Status
				Expect(obj.Status.ProxyRoutes).To(HaveLen(2))
				Expect(obj.Status.Route).ToNot(BeNil())
			})
		})

		// Scenario 3: Provider failover
		//
		// Exposure in zone-a with provider failover to zone-f.
		// One cross-zone subscriber in zone-b (not the failover zone).
		// Expects: 1 proxy route (zone-b with failover) + 1 secondary route (zone-f) + 1 real route.
		Context("provider failover: creates secondary route in failover zone", func() {
			It("creates a proxy route with failover, a secondary route, and a real route", func() {
				obj := newApiExposure(zoneARef)
				obj.Spec.Traffic = apiapi.Traffic{
					Failover: &apiapi.ProviderFailover{
						Zones: []ctypes.ObjectRef{zoneFRef},
					},
				}

				// --- Preamble ---
				mockListApis([]apiapi.Api{makeReadyApi()})
				mockListApiExposures([]apiapi.ApiExposure{*obj})
				mockGetApplication()
				mockGetTeamNotFound()

				// --- Step 1: determineRoutingState ---
				mockListSubscriptions([]apiapi.ApiSubscription{
					makeSubscription(basePath, zoneBRef, false),
				})

				// --- Step 2: manageProxyRoutes ---
				// Zone Gets: determineRoutingState(A) + LMS-collection(B) +
				//            downstream-proxy-B(B) + upstream-proxy-B(A) +
				//            addFailoverFallback(F) + downstream-secondary(F) + upstream-secondary(A) +
				//            real-route(A)
				mockGetZone(zoneARef.K8s(), zoneA(), 4) // determineRoutingState + upstream×2 + real route
				mockGetZone(zoneBRef.K8s(), zoneB(), 2) // LMS collection + downstream proxy
				mockGetZone(zoneFRef.K8s(), zoneF(), 3) // provider failover existence/feature check + addFailoverFallback + downstream secondary

				var routes []*gatewayapi.Route
				capturedRoutes(&routes, 3) // proxy + secondary + real

				mockCleanup()

				// --- Execute ---
				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// --- Assertions ---
				Expect(routes).To(HaveLen(3))

				// Proxy route for zone-b (index 0)
				proxyB := routes[0]
				Expect(proxyB.Namespace).To(Equal("ns-b"))
				Expect(proxyB.Spec.Type).To(Equal(gatewayapi.RouteTypeProxy))
				// Proxy route has failover traffic config pointing to zone-f
				Expect(proxyB.Spec.Traffic.Failover).ToNot(BeNil())
				Expect(proxyB.Spec.Traffic.Failover.TargetZoneName).To(Equal("zone-a"))
				Expect(proxyB.Spec.Security.RealmName).To(Equal(realmB))

				// Secondary route for zone-f (index 1)
				secondary := routes[1]
				Expect(secondary.Namespace).To(Equal("ns-f"))
				Expect(secondary.Spec.Type).To(Equal(gatewayapi.RouteTypeSecondary))
				Expect(secondary.Labels).To(HaveKeyWithValue("cp.ei.telekom.de/failover.secondary", "true"))
				// Secondary route gets gateway in DefaultConsumers
				Expect(secondary.Spec.Security.DefaultConsumers).To(ContainElement("gateway"))
				Expect(secondary.Spec.Security.RealmName).To(Equal(realmF))
				// Secondary route upstreams come from the exposure's upstreams
				Expect(secondary.Spec.Traffic.Failover).ToNot(BeNil())
				Expect(secondary.Spec.Traffic.Failover.Security.RealmName).To(Equal(realmF))

				// Real route (index 2)
				real := routes[2]
				Expect(real.Namespace).To(Equal("ns-a"))
				Expect(real.Spec.Type).To(Equal(gatewayapi.RouteTypePrimary))
				Expect(real.Spec.Security.DefaultConsumers).To(ContainElement("gateway"))
				Expect(real.Spec.Security.RealmName).To(Equal(realmA))
				// Real route trusts LMS issuers from cross-zone proxy zones
				Expect(real.Spec.Security.TrustedIssuers).To(ContainElement("https://lms.zone-b.example.com"))

				// Status
				Expect(obj.Status.ProxyRoutes).To(HaveLen(1))
				Expect(obj.Status.FailoverRoutes).ToNot(BeEmpty())
				Expect(obj.Status.Route).ToNot(BeNil())
			})
		})

		// Scenario: Zone change orphans the previous real route.
		//
		// The exposure lived in zone-b (Status.Route in ns-b) and moves to zone-a.
		// createRealRoute creates a new real route in ns-a; the stale real route in ns-b is
		// removed by the CleanupStaleRoutes sweep, which selects by basepath only so it also
		// covers real routes. The sweep must run only after the real route is created.
		Context("zone change: sweep cleans up the orphaned real route", func() {
			It("runs a basepath-only sweep after creating the new real route", func() {
				obj := newApiExposure(zoneARef)
				// Place the real route in zone-b's namespace to represent the pre-move state.
				obj.Status.Route = &ctypes.ObjectRef{
					Name:      util.MakeRouteName(basePath),
					Namespace: "ns-b",
				}

				// --- Preamble ---
				mockListApis([]apiapi.Api{makeReadyApi()})
				mockListApiExposures([]apiapi.ApiExposure{*obj})
				mockGetApplication()
				mockGetTeamNotFound()

				// --- Step 1: determineRoutingState (no subscribers → only real route) ---
				mockListSubscriptions([]apiapi.ApiSubscription{})

				// --- Step 2/3: Zone Gets for exposure zone (determineRoutingState + real route) ---
				mockGetZone(zoneARef.K8s(), zoneA(), 2)

				// Track ordering: the real route must be created before the sweep runs.
				var events []string

				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
					Run(func(_ context.Context, o client.Object, mutate controllerutil.MutateFn) {
						Expect(mutate()).To(Succeed())
						events = append(events, "create:"+o.(*gatewayapi.Route).Namespace)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				var cleanupOpts []client.ListOption
				fakeClient.EXPECT().
					Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
					Run(func(_ context.Context, _ ctypes.ObjectList, opts []client.ListOption) {
						events = append(events, "cleanup")
						cleanupOpts = opts
					}).
					Return(1, nil).Once()

				// --- Execute ---
				err := h.CreateOrUpdate(ctx, obj)
				Expect(err).ToNot(HaveOccurred())

				// --- Assertions ---
				// The real route was created before the cleanup sweep ran.
				Expect(events).To(Equal([]string{"create:ns-a", "cleanup"}))

				// The sweep selects by basepath only (no type filter), so it can also
				// delete orphaned real routes.
				Expect(cleanupOpts).To(HaveLen(1))
				matchingLabels, ok := cleanupOpts[0].(client.MatchingLabels)
				Expect(ok).To(BeTrue())
				Expect(matchingLabels).To(HaveKey(apiapi.BasePathLabelKey))
				Expect(matchingLabels).ToNot(HaveKey("cp.ei.telekom.de/type"))

				// Status now points at the new route in ns-a
				Expect(obj.Status.Route).ToNot(BeNil())
				Expect(obj.Status.Route.Namespace).To(Equal("ns-a"))
			})
		})
	})
})

var _ = Describe("ApiMustExist route cleanup", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, testEnv)
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	// When the corresponding Api is not registered/active, ApiMustExist cleans up all
	// routes for this basepath (proxy, real, and failover) across namespaces via the same
	// basepath-scoped sweep (CleanupStaleRoutes) the reconcile path uses.
	It("cleans up all routes by basepath when the Api is missing", func() {
		obj := newApiExposure(ctypes.ObjectRef{Name: "zone-a", Namespace: "ns-a"})

		// FindActiveAPI → empty list → API not found, triggering the cleanup path.
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiList"), mock.Anything, mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*apiapi.ApiList) = apiapi.ApiList{}
			}).
			Return(nil).Once()

		var cleanupOpts []client.ListOption
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Run(func(_ context.Context, _ ctypes.ObjectList, opts []client.ListOption) {
				cleanupOpts = opts
			}).
			Return(0, nil).Once()

		api, err := ApiMustExist(ctx, obj)
		Expect(err).ToNot(HaveOccurred())
		Expect(api).To(BeNil())

		// Cleanup must be basepath-scoped (matching the reconcile-time stale sweep) and must
		// NOT filter by type or owner.uid, so it removes proxy, real, and failover routes.
		Expect(cleanupOpts).To(HaveLen(1))
		matchingLabels, ok := cleanupOpts[0].(client.MatchingLabels)
		Expect(ok).To(BeTrue())
		Expect(matchingLabels).To(HaveKey(apiapi.BasePathLabelKey))
		Expect(matchingLabels).ToNot(HaveKey("cp.ei.telekom.de/type"))
		Expect(matchingLabels).ToNot(HaveKey("cp.ei.telekom.de/owner.uid"))
	})
})

var _ = Describe("ApiExposure Handler", func() {
	Context("validateApiCategoryPolicy", func() {
		const (
			environment = "test"
			group       = "alpha"
			teamName    = "core"
		)

		baseApp := &applicationapi.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "provider-app",
				Namespace: environment + "--" + group + "--" + teamName,
			},
		}
		baseAPI := &apiapi.Api{
			Spec: apiapi.ApiSpec{
				Category: "partner",
			},
		}

		type testCase struct {
			name           string
			teamCategory   organizationapi.TeamCategory
			apiCategories  []apiapi.ApiCategory
			expectedResult bool
			expectedReason string
		}

		tests := []testCase{
			{
				name:         "allowed category",
				teamCategory: organizationapi.TeamCategoryCustomer,
				apiCategories: []apiapi.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "partner", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiapi.ApiCategorySpec{
							LabelValue: "partner",
							Active:     true,
							AllowTeams: &apiapi.AllowTeamsConfig{Categories: []string{"Customer"}},
						},
					},
				},
				expectedResult: true,
			},
			{
				name:         "denied category",
				teamCategory: organizationapi.TeamCategoryInfrastructure,
				apiCategories: []apiapi.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "partner", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiapi.ApiCategorySpec{
							LabelValue: "partner",
							Active:     true,
							AllowTeams: &apiapi.AllowTeamsConfig{Categories: []string{"Customer"}},
						},
					},
				},
				expectedResult: false,
				expectedReason: util.ApiCategoryTeamCategoryNotAllowedReason,
			},
			{
				name:           "no categories configured",
				teamCategory:   organizationapi.TeamCategoryCustomer,
				apiCategories:  nil,
				expectedResult: true,
			},
			{
				name:         "missing category",
				teamCategory: organizationapi.TeamCategoryCustomer,
				apiCategories: []apiapi.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiapi.ApiCategorySpec{
							LabelValue: "other",
							Active:     true,
						},
					},
				},
				expectedResult: false,
				expectedReason: util.ApiCategoryPolicyResolutionFailedReason,
			},
			{
				name:         "inactive category",
				teamCategory: organizationapi.TeamCategoryCustomer,
				apiCategories: []apiapi.ApiCategory{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "partner", Namespace: environment, Labels: map[string]string{config.EnvironmentLabelKey: environment}},
						Spec: apiapi.ApiCategorySpec{
							LabelValue: "partner",
							Active:     false,
						},
					},
				},
				expectedResult: false,
				expectedReason: util.ApiCategoryPolicyResolutionFailedReason,
			},
		}

		for _, tt := range tests {
			It(tt.name, func() {
				team := &organizationapi.Team{
					ObjectMeta: metav1.ObjectMeta{
						Name:      group + "--" + teamName,
						Namespace: environment,
						Labels: map[string]string{
							config.EnvironmentLabelKey: environment,
						},
					},
					Spec: organizationapi.TeamSpec{
						Group:    group,
						Name:     teamName,
						Email:    "team@example.com",
						Category: tt.teamCategory,
					},
				}

				objects := []client.Object{team}
				for i := range tt.apiCategories {
					cat := tt.apiCategories[i]
					objects = append(objects, &cat)
				}

				ctx := newClientContext(environment, objects...)
				apiExp := &apiapi.ApiExposure{}

				result := validateApiCategoryPolicy(ctx, baseAPI, baseApp, apiExp)
				Expect(result).To(Equal(tt.expectedResult))

				if tt.expectedReason == "" {
					notReady := meta.FindStatusCondition(apiExp.GetConditions(), condition.ConditionTypeReady)
					Expect(notReady == nil || notReady.Status != metav1.ConditionFalse).To(BeTrue())
					return
				}

				notReady := meta.FindStatusCondition(apiExp.GetConditions(), condition.ConditionTypeReady)
				Expect(notReady).NotTo(BeNil())
				Expect(notReady.Reason).To(Equal(tt.expectedReason))
			})
		}
	})
})

func newClientContext(environment string, objects ...client.Object) context.Context {
	sch := runtime.NewScheme()
	Expect(apiapi.AddToScheme(sch)).To(Succeed())
	Expect(applicationapi.AddToScheme(sch)).To(Succeed())
	Expect(organizationapi.AddToScheme(sch)).To(Succeed())
	fakeClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(objects...).
		WithIndex(&apiapi.ApiCategory{}, "spec.labelValue", func(obj client.Object) []string {
			apiCategory, ok := obj.(*apiapi.ApiCategory)
			if !ok || apiCategory.Spec.LabelValue == "" {
				return nil
			}
			return []string{apiCategory.Spec.LabelValue}
		}).
		Build()
	janitorClient := cclient.NewJanitorClient(cclient.NewScopedClient(fakeClient, environment))
	return cclient.WithClient(context.Background(), janitorClient)
}
