// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticexposure_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	agenticconfig "github.com/telekom/controlplane/agentic/internal/config"
	"github.com/telekom/controlplane/agentic/internal/handler/agenticexposure"
	applicationapi "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newAgenticExposure(name, basePath string) *agenticv1.AgenticExposure {
	return &agenticv1.AgenticExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: agenticv1.AgenticExposureSpec{
			BasePath: basePath,
			Upstreams: []agenticv1.Upstream{
				{Url: "http://mcp-server.internal:8080"},
			},
			Visibility: agenticv1.VisibilityEnterprise,
			Approval:   agenticv1.Approval{Strategy: agenticv1.ApprovalStrategyAuto},
			Zone:       ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
			Provider:   ctypes.ObjectRef{Name: "test-app", Namespace: "default"},
			Variant:    agenticv1.AgenticVariantMCP,
		},
	}
}

func makeReadyAgenticServer(basePath string) agenticv1.AgenticServer {
	s := agenticv1.AgenticServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcp-server-1",
			Namespace: "default",
		},
		Spec: agenticv1.AgenticServerSpec{
			BasePath: basePath,
			Version:  "1.0.0",
			Name:     "Test MCP Server",
		},
		Status: agenticv1.AgenticServerStatus{
			Active: true,
		},
	}
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return s
}

func makeReadyZoneWithAiGateway() *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-zone",
			Namespace: "default",
		},
		Spec: adminv1.ZoneSpec{
			AiGateway: &adminv1.AiGatewayConfig{
				Presets: []adminv1.GatewayConfigPreset{
					{
						Name:    "default",
						Default: true,
						Urls: []adminv1.UrlConfig{
							{Hostname: "ai-gateway.example.com", Port: 443, Scheme: "https"},
						},
					},
				},
			},
		},
		Status: adminv1.ZoneStatus{
			Namespace: "default",
			AiGateway: &ctypes.ObjectRef{
				Name:      "ai-gateway",
				Namespace: "default",
			},
			Links: adminv1.Links{
				Issuer: "https://issuer.example.com",
			},
			Features: []adminv1.Feature{
				{Name: adminv1.FeatureAiGateway, Enabled: true},
			},
		},
	}
	meta.SetStatusCondition(&z.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return z
}

func makeZoneWithoutAiGateway() *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-zone",
			Namespace: "default",
		},
		Status: adminv1.ZoneStatus{
			Namespace: "default",
		},
	}
	meta.SetStatusCondition(&z.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return z
}

var zoneKey = k8stypes.NamespacedName{Name: "test-zone", Namespace: "default"}

var _ = Describe("AgenticExposureHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *agenticexposure.AgenticExposureHandler
		obj        *agenticv1.AgenticExposure
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &agenticexposure.AgenticExposureHandler{Config: &agenticconfig.AgenticConfig{}}
		obj = newAgenticExposure("test-exposure", "/mcp/weather/v1")
	})

	// --- mock helpers ---

	mockListAgenticServers := func(items []agenticv1.AgenticServer) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.AgenticServerList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgenticServerList) = agenticv1.AgenticServerList{Items: items}
			}).
			Return(nil).Once()
	}

	mockListAgenticServersError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.AgenticServerList"), mock.Anything).
			Return(err).Once()
	}

	mockListAgenticExposures := func(items []agenticv1.AgenticExposure) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.AgenticExposureList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgenticExposureList) = agenticv1.AgenticExposureList{Items: items}
			}).
			Return(nil).Once()
	}

	mockListAgenticExposuresError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.AgenticExposureList"), mock.Anything).
			Return(err).Once()
	}

	mockGetZone := func(zone *adminv1.Zone) {
		fakeClient.EXPECT().
			Get(ctx, zoneKey, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil).Once()
	}

	mockGetZoneError := func(err error) {
		fakeClient.EXPECT().
			Get(ctx, zoneKey, mock.AnythingOfType("*v1.Zone")).
			Return(err).Once()
	}

	mockListAgenticSubscriptions := func(items []agenticv1.AgenticSubscription) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.AgenticSubscriptionList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgenticSubscriptionList) = agenticv1.AgenticSubscriptionList{Items: items}
			}).
			Return(nil).Once()
	}

	mockCreateOrUpdateRoute := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	mockCleanup := func(deleted int, err error) {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(deleted, err).Once()
	}

	// setupFullHappyPath sets up all mocks for a successful CreateOrUpdate without cross-zone subscriptions.
	setupFullHappyPath := func() {
		server := makeReadyAgenticServer("/mcp/weather/v1")
		zone := makeReadyZoneWithAiGateway()

		mockListAgenticServers([]agenticv1.AgenticServer{server})
		mockListAgenticExposures([]agenticv1.AgenticExposure{})
		mockGetZone(zone)
		mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{}) // no cross-zone subs
		mockCreateOrUpdateRoute(controllerutil.OperationResultCreated, nil)
		mockCleanup(0, nil)
	}

	Describe("CreateOrUpdate", func() {
		It("should return error when FindActiveAgenticServer fails", func() {
			mockListAgenticServersError(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list AgenticServers"))
		})

		It("should set Blocked when no active AgenticServer found", func() {
			mockListAgenticServers([]agenticv1.AgenticServer{})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("AgenticServerNotFound"))
		})

		It("should set Blocked and clean up Route when AgenticServer disappears after Route was created", func() {
			// Simulate an exposure that already had a Route provisioned
			obj.Status.Route = &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}

			mockListAgenticServers([]agenticv1.AgenticServer{})

			// Expect the stale Route to be deleted (NotFound is fine — already gone)
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Route")).
				Return(apierrors.NewNotFound(schema.GroupResource{Resource: "routes"}, "ai-gateway--mcp-weather-v1")).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Reason).To(Equal("AgenticServerNotFound"))
		})

		It("should set Blocked with case-conflict reason when AgenticServer exists under different case", func() {
			// Server registered as /Mcp/Weather/V1 but exposure uses /mcp/weather/v1
			conflictingServer := makeReadyAgenticServer("/Mcp/Weather/V1")
			mockListAgenticServers([]agenticv1.AgenticServer{conflictingServer})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("AgenticServerCaseConflict"))
			Expect(readyCond.Message).To(ContainSubstring("/mcp/weather/v1"))
			Expect(readyCond.Message).To(ContainSubstring("/Mcp/Weather/V1"))
		})

		It("should return error when FindAgenticExposures fails", func() {
			server := makeReadyAgenticServer("/mcp/weather/v1")
			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposuresError(fmt.Errorf("list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list AgenticExposures"))
		})

		It("should set NotReady when another active AgenticExposure already exists", func() {
			server := makeReadyAgenticServer("/mcp/weather/v1")
			mockListAgenticServers([]agenticv1.AgenticServer{server})

			existingExposure := agenticv1.AgenticExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "other-exposure",
					Namespace:         "default",
					UID:               "other-uid",
					CreationTimestamp: metav1.Now(),
				},
				Spec: agenticv1.AgenticExposureSpec{
					BasePath: "/mcp/weather/v1",
					Provider: ctypes.ObjectRef{Name: "other-app", Namespace: "other-ns"},
				},
				Status: agenticv1.AgenticExposureStatus{
					Active: true,
				},
			}
			mockListAgenticExposures([]agenticv1.AgenticExposure{existingExposure})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("AgenticExposureAlreadyExists"))
		})

		It("should return error when GetZone fails", func() {
			server := makeReadyAgenticServer("/mcp/weather/v1")
			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZoneError(fmt.Errorf("zone fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("zone"))
		})

		It("should return BlockedError when zone does not support AI Gateway feature", func() {
			server := makeReadyAgenticServer("/mcp/weather/v1")
			zone := makeZoneWithoutAiGateway()

			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AI Gateway feature"))

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("AiGatewayNotSupported"))
		})

		It("should create Route and set Active=true when all is well", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
			Expect(obj.Status.Route).ToNot(BeNil())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("AgenticExposureProvisioned"))
		})

		It("should set NotReady when AllReady returns false", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(false).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ChildResourcesNotReady"))
		})

		It("should return error when CreateAgenticRoute fails", func() {
			server := makeReadyAgenticServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})
			mockCreateOrUpdateRoute(controllerutil.OperationResultNone, fmt.Errorf("route creation failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create MCP Route"))
		})

		It("should return error when Cleanup fails", func() {
			server := makeReadyAgenticServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})
			mockCreateOrUpdateRoute(controllerutil.OperationResultCreated, nil)
			mockCleanup(0, fmt.Errorf("cleanup failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to cleanup old MCP Routes"))
		})

		It("should fail when TELECONTEXTMCP variant is set but application ID is empty", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = ""

			server := makeReadyAgenticServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("telecontext application ID"))
		})

		It("should add Telecontext consumer to Route DefaultConsumers when variant is TELECONTEXTMCP", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")

			server := makeReadyAgenticServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})

			// Mock the Telecontext Application lookup — same zone as exposure
			telecontextApp := &applicationapi.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcapp",
					Namespace: "test-env--mcp--telecontext",
				},
				Spec: applicationapi.ApplicationSpec{
					Team:   "telecontext",
					Zone:   ctypes.ObjectRef{Name: "test-zone", Namespace: "test-env"},
					Secret: "test-secret",
				},
			}
			meta.SetStatusCondition(&telecontextApp.Status.Conditions, metav1.Condition{
				Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
			})
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
					mock.AnythingOfType("*v1.Application")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*applicationapi.Application) = *telecontextApp
				}).
				Return(nil).Once()

			var capturedRoute gatewayv1.Route
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					capturedRoute = *obj.(*gatewayv1.Route)
				}).
				Return(controllerutil.OperationResultCreated, nil).Once()

			mockCleanup(0, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(capturedRoute.Spec.Security.DefaultConsumers).To(ContainElement("telecontext--tcapp"))
			Expect(obj.Status.Route).ToNot(BeNil())
		})

		It("should create proxy route on Telecontext zone when it differs from exposure zone", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")

			server := makeReadyAgenticServer("/mcp/weather/v1")
			providerZone := makeReadyZoneWithAiGateway()
			providerZone.Status.Links.Issuer = "https://issuer.provider.example.com"

			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(providerZone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})

			// Mock the Telecontext Application lookup — DIFFERENT zone
			telecontextZone := makeReadyZoneWithAiGateway()
			telecontextZone.Name = "telecontext-zone"
			telecontextZone.Status.Namespace = "telecontext-zone-ns"
			telecontextZone.Status.Links.LmsIssuer = "https://lms.telecontext.example.com"
			telecontextZone.Status.Links.Issuer = "https://issuer.telecontext.example.com"

			telecontextApp := &applicationapi.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcapp",
					Namespace: "test-env--mcp--telecontext",
				},
				Spec: applicationapi.ApplicationSpec{
					Team:   "telecontext",
					Zone:   ctypes.ObjectRef{Name: "telecontext-zone", Namespace: "test-env"},
					Secret: "test-secret",
				},
			}
			meta.SetStatusCondition(&telecontextApp.Status.Conditions, metav1.Condition{
				Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
			})
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
					mock.AnythingOfType("*v1.Application")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*applicationapi.Application) = *telecontextApp
				}).
				Return(nil).Once()

			// Mock lookup of the Telecontext zone
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "telecontext-zone", Namespace: "test-env"},
					mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *telecontextZone
				}).
				Return(nil).Once()

			// First CreateOrUpdate: proxy route on Telecontext zone
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) { _ = mutate() }).
				Return(controllerutil.OperationResultCreated, nil).Once()

			// Second CreateOrUpdate: primary route on provider zone
			var capturedPrimaryRoute gatewayv1.Route
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					capturedPrimaryRoute = *obj.(*gatewayv1.Route)
				}).
				Return(controllerutil.OperationResultCreated, nil).Once()

			mockCleanup(0, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			// Proxy route created for the Telecontext zone
			Expect(obj.Status.ProxyRoutes).To(HaveLen(1))
			// Telecontext zone's LMS issuer is trusted on the primary route
			Expect(capturedPrimaryRoute.Spec.Security.TrustedIssuers).To(ContainElement("https://lms.telecontext.example.com"))
			// Telecontext consumer is on the primary route
			Expect(capturedPrimaryRoute.Spec.Security.DefaultConsumers).To(ContainElement("telecontext--tcapp"))
			// isProxyTarget should be true → gateway mesh-client also added
			Expect(capturedPrimaryRoute.Spec.Security.DefaultConsumers).To(ContainElement("gateway"))
		})

		It("should add cross-zone LMS issuer to TrustedIssuers on the real route (no local subs — zone issuer excluded)", func() {
			server := makeReadyAgenticServer("/mcp/weather/v1")
			providerZone := makeReadyZoneWithAiGateway()
			providerZone.Status.Links.Issuer = "https://issuer.provider.example.com"

			subscriberZone := makeReadyZoneWithAiGateway()
			subscriberZone.Name = "subscriber-zone"
			subscriberZone.Status.Links.LmsIssuer = "https://lms.subscriber.example.com"

			approvedSub := agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub-1", Namespace: "default"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath: "/mcp/weather/v1",
					Zone:     ctypes.ObjectRef{Name: "subscriber-zone", Namespace: "default"},
				},
			}
			meta.SetStatusCondition(&approvedSub.Status.Conditions, metav1.Condition{
				Type: "ApprovalGranted", Status: metav1.ConditionTrue, Reason: "Approved",
			})

			mockListAgenticServers([]agenticv1.AgenticServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(providerZone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{approvedSub})

			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "subscriber-zone", Namespace: "default"},
					mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *subscriberZone
				}).
				Return(nil).Once()

			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) { _ = mutate() }).
				Return(controllerutil.OperationResultCreated, nil).Once()

			var capturedRoute gatewayv1.Route
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					capturedRoute = *obj.(*gatewayv1.Route)
				}).
				Return(controllerutil.OperationResultCreated, nil).Once()

			mockCleanup(0, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			// LMS issuer from the proxy zone IS trusted
			Expect(capturedRoute.Spec.Security.TrustedIssuers).To(ContainElement("https://lms.subscriber.example.com"))
			// No local subs → the provider zone's own IDP issuer is NOT added
			Expect(capturedRoute.Spec.Security.TrustedIssuers).NotTo(ContainElement("https://issuer.provider.example.com"))
		})
	}) // end Describe("CreateOrUpdate")

	Describe("Delete", func() {
		It("should skip Route deletion when another AgenticExposure exists", func() {
			obj.Status.Route = &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}

			// AnyOtherAgenticExposureExists calls FindAgenticExposures which lists
			otherExposure := agenticv1.AgenticExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-exposure",
					Namespace: "default",
					UID:       "other-uid",
				},
				Spec: agenticv1.AgenticExposureSpec{
					BasePath: "/mcp/weather/v1",
				},
			}
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgenticExposureList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgenticExposureList) = agenticv1.AgenticExposureList{
						Items: []agenticv1.AgenticExposure{otherExposure},
					}
				}).
				Return(nil).Once()

			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should delete Route when no other AgenticExposure exists", func() {
			obj.Status.Route = &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}

			// No other exposures
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.AgenticExposureList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.AgenticExposureList) = agenticv1.AgenticExposureList{Items: []agenticv1.AgenticExposure{}}
				}).
				Return(nil).Once()

			// Delete Route
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Route")).
				Return(nil).Once()

			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})
	})
})
