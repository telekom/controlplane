// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpexposure_test

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
	"github.com/telekom/controlplane/agentic/internal/handler/mcpexposure"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func newMcpExposure(name, basePath string) *agenticv1.McpExposure {
	return &agenticv1.McpExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: agenticv1.McpExposureSpec{
			BasePath: basePath,
			Upstreams: []agenticv1.Upstream{
				{Url: "http://mcp-server.internal:8080"},
			},
			Visibility: agenticv1.VisibilityEnterprise,
			Approval:   agenticv1.Approval{Strategy: agenticv1.ApprovalStrategyAuto},
			Zone:       ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
			Provider:   ctypes.ObjectRef{Name: "test-app", Namespace: "default"},
			Variant:    agenticv1.McpVariantMCP,
		},
	}
}

//nolint:unparam // test helper designed for reuse with different basePaths
func makeReadyMcpServer(basePath string) agenticv1.McpServer {
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

var (
	zoneKey = k8stypes.NamespacedName{Name: "test-zone", Namespace: "default"}
)

var _ = Describe("McpExposureHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *mcpexposure.McpExposureHandler
		obj        *agenticv1.McpExposure
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &mcpexposure.McpExposureHandler{Config: &agenticconfig.AgenticConfig{}}
		obj = newMcpExposure("test-exposure", "/mcp/weather/v1")
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

	mockListMcpExposures := func(items []agenticv1.McpExposure) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.McpExposureList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.McpExposureList) = agenticv1.McpExposureList{Items: items}
			}).
			Return(nil).Once()
	}

	mockListMcpExposuresError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.McpExposureList"), mock.Anything).
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

	mockListMcpSubscriptions := func(items []agenticv1.McpSubscription) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.McpSubscriptionList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*agenticv1.McpSubscriptionList) = agenticv1.McpSubscriptionList{Items: items}
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
		mockListMcpExposures([]agenticv1.McpExposure{})
		mockGetZone(zone)
		mockListMcpSubscriptions([]agenticv1.McpSubscription{}) // no cross-zone subs
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

		It("should set Blocked when no active McpServer found", func() {
			mockListMcpServers([]agenticv1.McpServer{})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("McpServerNotFound"))
		})

		It("should set Blocked and clean up Route when McpServer disappears after Route was created", func() {
			// Simulate an exposure that already had a Route provisioned
			obj.Status.Route = &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}

			mockListMcpServers([]agenticv1.McpServer{})

			// Expect the stale Route to be deleted (NotFound is fine — already gone)
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Route")).
				Return(apierrors.NewNotFound(schema.GroupResource{Resource: "routes"}, "ai-gateway--mcp-weather-v1")).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Reason).To(Equal("McpServerNotFound"))
		})

		It("should set Blocked with case-conflict reason when McpServer exists under different case", func() {
			// Server registered as /Mcp/Weather/V1 but exposure uses /mcp/weather/v1
			conflictingServer := makeReadyMcpServer("/Mcp/Weather/V1")
			mockListMcpServers([]agenticv1.McpServer{conflictingServer})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("McpServerCaseConflict"))
			Expect(readyCond.Message).To(ContainSubstring("/mcp/weather/v1"))
			Expect(readyCond.Message).To(ContainSubstring("/Mcp/Weather/V1"))
		})

		It("should return error when FindMcpExposures fails", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposuresError(fmt.Errorf("list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list McpExposures"))
		})

		It("should set NotReady when another active McpExposure already exists", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			mockListMcpServers([]agenticv1.McpServer{server})

			existingExposure := agenticv1.McpExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "other-exposure",
					Namespace:         "default",
					UID:               "other-uid",
					CreationTimestamp: metav1.Now(),
				},
				Spec: agenticv1.McpExposureSpec{
					BasePath: "/mcp/weather/v1",
					Provider: ctypes.ObjectRef{Name: "other-app", Namespace: "other-ns"},
				},
				Status: agenticv1.McpExposureStatus{
					Active: true,
				},
			}
			mockListMcpExposures([]agenticv1.McpExposure{existingExposure})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("McpExposureAlreadyExists"))
		})

		It("should return error when GetZone fails", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})
			mockGetZoneError(fmt.Errorf("zone fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("zone"))
		})

		It("should return BlockedError when zone does not support AI Gateway feature", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeZoneWithoutAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})
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
			Expect(readyCond.Reason).To(Equal("McpExposureProvisioned"))
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

		It("should return error when CreateMcpRoute fails", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})
			mockGetZone(zone)
			mockListMcpSubscriptions([]agenticv1.McpSubscription{})
			mockCreateOrUpdateRoute(controllerutil.OperationResultNone, fmt.Errorf("route creation failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create MCP Route"))
		})

		It("should return error when Cleanup fails", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})
			mockGetZone(zone)
			mockListMcpSubscriptions([]agenticv1.McpSubscription{})
			mockCreateOrUpdateRoute(controllerutil.OperationResultCreated, nil)
			mockCleanup(0, fmt.Errorf("cleanup failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to cleanup old MCP Routes"))
		})

		It("should fail when TELECONTEXTMCP variant is set but consumer name is empty", func() {
			obj.Spec.Variant = agenticv1.McpVariantTelecontextMCP
			h.Config.TelecontextConsumerName = ""

			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})
			mockGetZone(zone)
			mockListMcpSubscriptions([]agenticv1.McpSubscription{})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("telecontext consumer name"))
		})

		It("should add Telecontext consumer to Route DefaultConsumers when variant is TELECONTEXTMCP", func() {
			obj.Spec.Variant = agenticv1.McpVariantTelecontextMCP
			h.Config.TelecontextConsumerName = "telecontext-app"

			server := makeReadyMcpServer("/mcp/weather/v1")
			zone := makeReadyZoneWithAiGateway()

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})
			mockGetZone(zone)
			mockListMcpSubscriptions([]agenticv1.McpSubscription{})

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
			Expect(capturedRoute.Spec.Security.DefaultConsumers).To(ContainElement("telecontext-app"))
			Expect(obj.Status.Route).ToNot(BeNil())
		})

		It("should add cross-zone LMS issuer to TrustedIssuers on the real route", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			providerZone := makeReadyZoneWithAiGateway()
			providerZone.Status.Links.Issuer = "https://issuer.provider.example.com"

			subscriberZone := makeReadyZoneWithAiGateway()
			subscriberZone.Name = "subscriber-zone"
			subscriberZone.Status.Links.LmsIssuer = "https://lms.subscriber.example.com"

			approvedSub := agenticv1.McpSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sub-1", Namespace: "default",
				},
				Spec: agenticv1.McpSubscriptionSpec{
					BasePath: "/mcp/weather/v1",
					Zone:     ctypes.ObjectRef{Name: "subscriber-zone", Namespace: "default"},
				},
			}
			meta.SetStatusCondition(&approvedSub.Status.Conditions, metav1.Condition{
				Type: "ApprovalGranted", Status: metav1.ConditionTrue, Reason: "Approved",
			})

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})
			mockGetZone(providerZone) // step 3 — provider zone
			mockListMcpSubscriptions([]agenticv1.McpSubscription{approvedSub})

			// GetZone for subscriber zone (in proxy route loop)
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: "subscriber-zone", Namespace: "default"},
					mock.AnythingOfType("*v1.Zone")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					*out.(*adminv1.Zone) = *subscriberZone
				}).
				Return(nil).Once()

			// proxy route CreateOrUpdate (in subscriber zone namespace)
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
				Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) { _ = mutate() }).
				Return(controllerutil.OperationResultCreated, nil).Once()

			// real route CreateOrUpdate — capture to inspect TrustedIssuers
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
			Expect(capturedRoute.Spec.Security.TrustedIssuers).To(ContainElement("https://lms.subscriber.example.com"))
			Expect(capturedRoute.Spec.Security.TrustedIssuers).To(ContainElement("https://issuer.provider.example.com"))
		})

	}) // end Describe("CreateOrUpdate")

	Describe("Delete", func() {
		It("should skip Route deletion when another McpExposure exists", func() {
			obj.Status.Route = &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}

			// AnyOtherMcpExposureExists calls FindMcpExposures which lists
			otherExposure := agenticv1.McpExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-exposure",
					Namespace: "default",
					UID:       "other-uid",
				},
				Spec: agenticv1.McpExposureSpec{
					BasePath: "/mcp/weather/v1",
				},
			}
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpExposureList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpExposureList) = agenticv1.McpExposureList{
						Items: []agenticv1.McpExposure{otherExposure},
					}
				}).
				Return(nil).Once()

			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should delete Route when no other McpExposure exists", func() {
			obj.Status.Route = &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"}

			// No other exposures
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.McpExposureList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*agenticv1.McpExposureList) = agenticv1.McpExposureList{Items: []agenticv1.McpExposure{}}
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
