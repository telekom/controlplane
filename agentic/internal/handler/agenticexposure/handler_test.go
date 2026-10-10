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
	"github.com/telekom/controlplane/common/pkg/config"
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

func makeReadyMcpServer(basePath string) agenticv1.McpServer { //nolint:unparam // test helper kept parameterized for clarity
	s := agenticv1.McpServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcp-server-1",
			Namespace: "default",
		},
		Spec: agenticv1.McpServerSpec{
			BasePath: basePath,
			Version:  "1.0.0",
			Name:     "Test MCP Server",
		},
		Status: agenticv1.McpServerStatus{
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
			Gateways: []adminv1.GatewayConfig{
				{Name: "ai"},
			},
			Presets: []adminv1.Preset{
				{
					Name:       "default",
					Type:       adminv1.GatewayTypeAI,
					Default:    true,
					GatewayRef: "ai",
					Urls: []adminv1.UrlConfig{
						{Hostname: "ai-gateway.example.com", Port: 443, Scheme: "https"},
					},
				},
				// Every zone needs an API preset as its representative profile; agentic
				// selection never resolves through it.
				{
					Name:       "api-default",
					Type:       adminv1.GatewayTypeAPI,
					Default:    true,
					GatewayRef: "ai",
					Urls: []adminv1.UrlConfig{
						{Hostname: "ai-gateway.example.com", Port: 443, Scheme: "https"},
					},
				},
			},
		},
		Status: adminv1.ZoneStatus{
			Namespace: "default",
			RealmName: "provider-realm",
			Presets: []adminv1.PresetStatus{{Name: "default", GatewayRef: &ctypes.ObjectRef{
				Name: "ai-gateway", Namespace: "default",
			}, Links: adminv1.Links{Issuer: "https://issuer.example.com"}}},
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

	mockListMcpServers := func(items []agenticv1.McpServer) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.McpServerList) = agenticv1.McpServerList{Items: items}
			}).
			Return(nil).Once()
	}

	mockListMcpServersError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.McpServerList"), mock.Anything).
			Return(err).Once()
	}

	mockListAgentCards := func(items []agenticv1.AgentCard) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.AgentCardList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.AgentCardList) = agenticv1.AgentCardList{Items: items}
			}).
			Return(nil).Once()
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
		server := makeReadyMcpServer("/mcp/weather/v1")
		zone := makeReadyZoneWithAiGateway()

		mockListMcpServers([]agenticv1.McpServer{server})
		mockListAgenticExposures([]agenticv1.AgenticExposure{})
		mockGetZone(zone)
		mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{}) // no cross-zone subs
		mockCreateOrUpdateRoute(controllerutil.OperationResultCreated, nil)
		mockCleanup(0, nil)
	}

	Describe("CreateOrUpdate", func() {
		It("should return error when FindActiveMcpServer fails", func() {
			mockListMcpServersError(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list McpServers"))
		})

		It("should set Blocked when no active server found", func() {
			mockListMcpServers([]agenticv1.McpServer{})
			mockListAgentCards([]agenticv1.AgentCard{})
			// ServerMustExist does additional case-conflict lookups when not found
			mockListMcpServers([]agenticv1.McpServer{})
			mockListAgentCards([]agenticv1.AgentCard{})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ServerNotFound"))
		})

		It("should set Blocked and clean up Route when server disappears after Route was created", func() {
			obj.Status.Route = &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}

			mockListMcpServers([]agenticv1.McpServer{})
			mockListAgentCards([]agenticv1.AgentCard{})
			// ServerMustExist does additional case-conflict lookups when not found
			mockListMcpServers([]agenticv1.McpServer{})
			mockListAgentCards([]agenticv1.AgentCard{})

			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Route")).
				Return(apierrors.NewNotFound(schema.GroupResource{Resource: "routes"}, "ai-gateway--mcp-weather-v1")).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Reason).To(Equal("ServerNotFound"))
		})

		It("should return error when FindAgenticExposures fails", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposuresError(fmt.Errorf("list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list AgenticExposures"))
		})

		It("should set NotReady when another active AgenticExposure already exists", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			mockListMcpServers([]agenticv1.McpServer{server})

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
			server := makeReadyMcpServer("/mcp/weather/v1")
			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZoneError(fmt.Errorf("zone fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("zone"))
		})

		It("should return BlockedError when zone does not support AI Gateway feature", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeZoneWithoutAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
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
			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()
			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})
			var capturedRoute gatewayv1.Route
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, route client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					capturedRoute = *route.(*gatewayv1.Route)
				}).
				Return(controllerutil.OperationResultCreated, nil).Once()
			mockCleanup(0, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeTrue())
			Expect(obj.Status.Route).ToNot(BeNil())
			Expect(capturedRoute.Spec.Security.RealmName).To(Equal("provider-realm"))

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
			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})
			mockCreateOrUpdateRoute(controllerutil.OperationResultNone, fmt.Errorf("route creation failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create MCP Route"))
		})

		It("should return error when Cleanup fails", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
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

			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("telecontext application ID"))
		})

		It("should add Telecontext consumer to Route DefaultConsumers when variant is TELECONTEXTMCP", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")

			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
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
					Team:   "mcp--telecontext",
					Zone:   ctypes.ObjectRef{Name: "test-zone", Namespace: "test-env"},
					Secret: "test-secret",
				},
				Status: applicationapi.ApplicationStatus{ClientId: "mcp--telecontext--tcapp"},
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
			Expect(capturedRoute.Spec.Security.DefaultConsumers).To(ConsistOf("mcp--telecontext--tcapp"))
			// Telecontext calls the route directly, so the zone's own issuer is trusted
			Expect(capturedRoute.Spec.Security.TrustedIssuers).To(ConsistOf("https://issuer.example.com"))
			Expect(capturedRoute.Spec.AdditionalTags).To(Equal([]string{"variant--telecontextmcp"}))
			Expect(obj.Status.Route).ToNot(BeNil())
		})

		It("should be blocked when the Telecontext Application has no client ID", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")
			previousRoute := &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}
			previousProxyRoutes := []ctypes.ObjectRef{{Name: "ai-gateway--mcp-weather-v1", Namespace: "subscriber-zone-ns"}}
			obj.Status.Route = previousRoute
			obj.Status.ProxyRoutes = previousProxyRoutes

			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)

			telecontextApp := &applicationapi.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
				Spec: applicationapi.ApplicationSpec{
					Team: "mcp--telecontext", Zone: ctypes.ObjectRef{Name: "test-zone", Namespace: "test-env"}, Secret: "test-secret",
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

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("has no client ID"))
			// The routes of the previous reconcile stay in the status
			Expect(obj.Status.Route).To(Equal(previousRoute))
			Expect(obj.Status.ProxyRoutes).To(Equal(previousProxyRoutes))
		})

		It("should be blocked when the Telecontext Application does not exist", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")
			previousRoute := &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}
			previousProxyRoutes := []ctypes.ObjectRef{{Name: "ai-gateway--mcp-weather-v1", Namespace: "subscriber-zone-ns"}}
			obj.Status.Route = previousRoute
			obj.Status.ProxyRoutes = previousProxyRoutes

			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
					mock.AnythingOfType("*v1.Application")).
				Return(apierrors.NewNotFound(schema.GroupResource{Group: "application.cp.ei.telekom.de", Resource: "applications"}, "tcapp")).
				Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			// The routes of the previous reconcile stay in the status
			Expect(obj.Status.Route).To(Equal(previousRoute))
			Expect(obj.Status.ProxyRoutes).To(Equal(previousProxyRoutes))
		})

		It("should create proxy route on Telecontext zone when it differs from exposure zone", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")

			server := makeReadyMcpServer("/mcp/weather/v1")
			providerZone := makeReadyZoneWithAiGateway()
			providerZone.Status.Presets[0].Links.Issuer = "https://issuer.provider.example.com"

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(providerZone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})

			// Mock the Telecontext Application lookup — DIFFERENT zone
			telecontextZone := makeReadyZoneWithAiGateway()
			telecontextZone.Name = "telecontext-zone"
			telecontextZone.Status.Namespace = "telecontext-zone-ns"
			telecontextZone.Status.RealmName = "telecontext-realm"
			telecontextZone.Status.Presets[0].Links.LmsIssuer = "https://lms.telecontext.example.com"
			telecontextZone.Status.Presets[0].Links.Issuer = "https://issuer.telecontext.example.com"

			telecontextApp := &applicationapi.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcapp",
					Namespace: "test-env--mcp--telecontext",
				},
				Spec: applicationapi.ApplicationSpec{
					Team:   "mcp--telecontext",
					Zone:   ctypes.ObjectRef{Name: "telecontext-zone", Namespace: "test-env"},
					Secret: "test-secret",
				},
				Status: applicationapi.ApplicationStatus{ClientId: "mcp--telecontext--tcapp"},
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
			var capturedProxyRoute gatewayv1.Route
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, route client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					capturedProxyRoute = *route.(*gatewayv1.Route)
				}).
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
			Expect(capturedProxyRoute.Spec.Security.RealmName).To(Equal("telecontext-realm"))
			Expect(capturedPrimaryRoute.Spec.Security.RealmName).To(Equal("provider-realm"))
			Expect(capturedProxyRoute.Spec.AdditionalTags).To(Equal([]string{"variant--telecontextmcp"}))
			Expect(capturedPrimaryRoute.Spec.AdditionalTags).To(Equal([]string{"variant--telecontextmcp"}))
			// Telecontext zone's LMS issuer is trusted on the primary route
			Expect(capturedPrimaryRoute.Spec.Security.TrustedIssuers).To(ContainElement("https://lms.telecontext.example.com"))
			// Telecontext calls the proxy route in its own zone with a token of that zone
			Expect(capturedProxyRoute.Spec.Security.DefaultConsumers).To(ConsistOf("gateway", "mcp--telecontext--tcapp"))
			Expect(capturedProxyRoute.Spec.Security.TrustedIssuers).To(ConsistOf("https://issuer.telecontext.example.com"))
			// The primary route only sees the gateway mesh client
			Expect(capturedPrimaryRoute.Spec.Security.DefaultConsumers).To(ConsistOf("gateway"))
			Expect(capturedPrimaryRoute.Spec.Security.TrustedIssuers).NotTo(ContainElement("https://issuer.provider.example.com"))
		})

		It("should add the Telecontext consumer only to the proxy route in its own zone when another zone subscribes", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")

			server := makeReadyMcpServer("/mcp/weather/v1")
			providerZone := makeReadyZoneWithAiGateway()

			subscriberZone := makeReadyZoneWithAiGateway()
			subscriberZone.Name = "subscriber-zone"
			subscriberZone.Status.Namespace = "subscriber-zone-ns"
			subscriberZone.Status.Presets[0].Links.LmsIssuer = "https://lms.subscriber.example.com"

			telecontextZone := makeReadyZoneWithAiGateway()
			telecontextZone.Name = "telecontext-zone"
			telecontextZone.Status.Namespace = "telecontext-zone-ns"
			telecontextZone.Status.Presets[0].Links.LmsIssuer = "https://lms.telecontext.example.com"

			telecontextApp := &applicationapi.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
				Spec: applicationapi.ApplicationSpec{
					Team: "mcp--telecontext", Zone: ctypes.ObjectRef{Name: "telecontext-zone", Namespace: "default"}, Secret: "test-secret",
				},
				Status: applicationapi.ApplicationStatus{ClientId: "mcp--telecontext--tcapp"},
			}
			meta.SetStatusCondition(&telecontextApp.Status.Conditions, metav1.Condition{
				Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
			})

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

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(providerZone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{approvedSub})
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
					mock.AnythingOfType("*v1.Application")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*applicationapi.Application) = *telecontextApp
				}).
				Return(nil).Once()
			for _, z := range []*adminv1.Zone{subscriberZone, telecontextZone} {
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: z.Name, Namespace: "default"}, mock.AnythingOfType("*v1.Zone")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*adminv1.Zone) = *z
					}).
					Return(nil).Once()
			}

			routes := map[string]gatewayv1.Route{}
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, route client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					routes[route.GetNamespace()] = *route.(*gatewayv1.Route)
				}).
				Return(controllerutil.OperationResultCreated, nil).Times(3)
			mockCleanup(0, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.ProxyRoutes).To(HaveLen(2))
			Expect(routes).To(HaveLen(3))
			Expect(routes["subscriber-zone-ns"].Spec.Security.DefaultConsumers).To(ConsistOf("gateway"))
			Expect(routes["telecontext-zone-ns"].Spec.Security.DefaultConsumers).To(ConsistOf("gateway", "mcp--telecontext--tcapp"))
			Expect(routes["default"].Spec.Security.DefaultConsumers).To(ConsistOf("gateway"))
			// The primary route trusts the LMS issuer of each proxy zone once
			Expect(routes["default"].Spec.Security.TrustedIssuers).To(ConsistOf(
				"https://lms.subscriber.example.com", "https://lms.telecontext.example.com"))
		})

		It("should add the Telecontext consumer to the primary route when Telecontext is local and another zone subscribes", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantTelecontextMCP
			h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
			ctx = contextutil.WithEnv(ctx, "test-env")

			server := makeReadyMcpServer("/mcp/weather/v1")
			providerZone := makeReadyZoneWithAiGateway()

			subscriberZone := makeReadyZoneWithAiGateway()
			subscriberZone.Name = "subscriber-zone"
			subscriberZone.Status.Namespace = "subscriber-zone-ns"
			subscriberZone.Status.Presets[0].Links.LmsIssuer = "https://lms.subscriber.example.com"

			// Telecontext is in the exposure zone
			telecontextApp := &applicationapi.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
				Spec: applicationapi.ApplicationSpec{
					Team: "mcp--telecontext", Zone: ctypes.ObjectRef{Name: "test-zone", Namespace: "default"}, Secret: "test-secret",
				},
				Status: applicationapi.ApplicationStatus{ClientId: "mcp--telecontext--tcapp"},
			}
			meta.SetStatusCondition(&telecontextApp.Status.Conditions, metav1.Condition{
				Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
			})

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

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(providerZone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{approvedSub})
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
					mock.AnythingOfType("*v1.Application")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*applicationapi.Application) = *telecontextApp
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "subscriber-zone", Namespace: "default"}, mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *subscriberZone
				}).
				Return(nil).Once()

			routes := map[string]gatewayv1.Route{}
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, route client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					routes[route.GetNamespace()] = *route.(*gatewayv1.Route)
				}).
				Return(controllerutil.OperationResultCreated, nil).Times(2)
			mockCleanup(0, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.ProxyRoutes).To(HaveLen(1))
			Expect(routes).To(HaveLen(2))
			Expect(routes["subscriber-zone-ns"].Spec.Security.DefaultConsumers).To(ConsistOf("gateway"))
			// The primary route serves the proxy gateway and Telecontext directly
			Expect(routes["default"].Spec.Security.DefaultConsumers).To(ConsistOf("gateway", "mcp--telecontext--tcapp"))
			Expect(routes["default"].Spec.Security.TrustedIssuers).To(ConsistOf(
				"https://issuer.example.com", "https://lms.subscriber.example.com"))
		})

		DescribeTable("should preserve the variant and cross-zone trusted issuers on provider and subscriber routes", func(variant agenticv1.AgenticVariant, expectedTags []string) {
			obj.Spec.Variant = variant
			ctx = contextutil.WithEnv(ctx, "test-env")
			if variant.IsTelecontextVariant() {
				h.Config.TelecontextApplicationID = "mcp--telecontext--tcapp"
				// Telecontext is in the subscriber zone, so the subscription's proxy route serves it too
				telecontextApp := &applicationapi.Application{
					ObjectMeta: metav1.ObjectMeta{Name: "tcapp", Namespace: "test-env--mcp--telecontext"},
					Spec: applicationapi.ApplicationSpec{
						Team: "mcp--telecontext", Zone: ctypes.ObjectRef{Name: "subscriber-zone", Namespace: "default"}, Secret: "test-secret",
					},
					Status: applicationapi.ApplicationStatus{ClientId: "mcp--telecontext--tcapp"},
				}
				meta.SetStatusCondition(&telecontextApp.Status.Conditions, metav1.Condition{
					Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
				})
				fakeClient.EXPECT().Get(ctx, k8stypes.NamespacedName{Name: "tcapp", Namespace: "test-env--mcp--telecontext"}, mock.AnythingOfType("*v1.Application")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*applicationapi.Application) = *telecontextApp
					}).Return(nil).Once()
			}
			server := makeReadyMcpServer("/mcp/weather/v1")
			providerZone := makeReadyZoneWithAiGateway()
			providerZone.Status.Presets[0].Links.Issuer = "https://issuer.provider.example.com"

			subscriberZone := makeReadyZoneWithAiGateway()
			subscriberZone.Name = "subscriber-zone"
			subscriberZone.Status.RealmName = "subscriber-realm"
			subscriberZone.Status.Presets[0].Links.LmsIssuer = "https://lms.subscriber.example.com"

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

			mockListMcpServers([]agenticv1.McpServer{server})
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

			var capturedProxyRoute gatewayv1.Route
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, route client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
					capturedProxyRoute = *route.(*gatewayv1.Route)
				}).
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
			Expect(capturedProxyRoute.Spec.Security.RealmName).To(Equal("subscriber-realm"))
			Expect(capturedRoute.Spec.Security.RealmName).To(Equal("provider-realm"))
			Expect(capturedProxyRoute.Spec.AdditionalTags).To(Equal(expectedTags))
			Expect(capturedRoute.Spec.AdditionalTags).To(Equal(expectedTags))
			Expect(capturedProxyRoute.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "mcp-proxy"))
			Expect(capturedRoute.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "mcp"))
			// LMS issuer from the proxy zone IS trusted
			Expect(capturedRoute.Spec.Security.TrustedIssuers).To(ContainElement("https://lms.subscriber.example.com"))
			// No local subs → the provider zone's own IDP issuer is NOT added
			Expect(capturedRoute.Spec.Security.TrustedIssuers).NotTo(ContainElement("https://issuer.provider.example.com"))
			Expect(capturedRoute.Spec.Security.DefaultConsumers).To(ConsistOf("gateway"))
			// One proxy route per zone, also when Telecontext is in the subscriber zone
			Expect(obj.Status.ProxyRoutes).To(HaveLen(1))
			if variant.IsTelecontextVariant() {
				Expect(capturedProxyRoute.Spec.Security.DefaultConsumers).To(ConsistOf("gateway", "mcp--telecontext--tcapp"))
			} else {
				Expect(capturedProxyRoute.Spec.Security.DefaultConsumers).To(ConsistOf("gateway"))
			}
		},
			Entry("MCP", agenticv1.AgenticVariantMCP, []string{"variant--mcp"}),
			Entry("Telecontext MCP", agenticv1.AgenticVariantTelecontextMCP, []string{"variant--telecontextmcp"}),
			Entry("AGENT", agenticv1.AgenticVariantAgent, []string{"variant--agent"}),
		)

		It("should create Route for AGENT variant without triggering Telecontext logic", func() {
			obj.Spec.Variant = agenticv1.AgenticVariantAgent

			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListAgenticExposures([]agenticv1.AgenticExposure{})
			mockGetZone(zone)
			mockListAgenticSubscriptions([]agenticv1.AgenticSubscription{})

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
			Expect(obj.Status.Active).To(BeTrue())
			Expect(obj.Status.Route).ToNot(BeNil())
			// Route should have no DefaultConsumers (no Telecontext consumer)
			Expect(capturedRoute.Spec.Security.DefaultConsumers).To(BeEmpty())
			Expect(capturedRoute.Spec.AdditionalTags).To(Equal([]string{"variant--agent"}))
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
