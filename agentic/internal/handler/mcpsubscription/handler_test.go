// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package mcpsubscription_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	"github.com/telekom/controlplane/agentic/internal/handler/mcpsubscription"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func isBlockedError(err error) bool {
	var be ctrlerrors.BlockedError
	return errors.As(err, &be) && be.IsBlocked()
}

func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = adminv1.AddToScheme(s)
	_ = agenticv1.AddToScheme(s)
	_ = applicationv1.AddToScheme(s)
	_ = approvalv1.AddToScheme(s)
	return s
}

func newMcpSubscription(name, basePath, zoneName string) *agenticv1.McpSubscription {
	return &agenticv1.McpSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "sub-uid-1234",
		},
		Spec: agenticv1.McpSubscriptionSpec{
			McpBasePath: basePath,
			Zone:        ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			Requestor: ctypes.TypedObjectRef{
				ObjectRef: ctypes.ObjectRef{Name: "requestor-app", Namespace: "default"},
			},
		},
	}
}

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

func makeReadyMcpExposure(basePath, zoneName string) agenticv1.McpExposure {
	exp := agenticv1.McpExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-exposure",
			Namespace:         "default",
			UID:               "exposure-uid",
			CreationTimestamp: metav1.Now(),
		},
		Spec: agenticv1.McpExposureSpec{
			McpBasePath: basePath,
			Visibility:  agenticv1.VisibilityEnterprise,
			Approval:    agenticv1.Approval{Strategy: agenticv1.ApprovalStrategyAuto},
			Zone:        ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			Provider:    ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "provider-app", Namespace: "default"}},
			Variant:     agenticv1.McpVariantMCP,
		},
		Status: agenticv1.McpExposureStatus{
			Active: true,
			Route:  &ctypes.ObjectRef{Name: "ai-gateway--mcp-weather-v1", Namespace: "default"},
		},
	}
	meta.SetStatusCondition(&exp.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return exp
}

func makeReadyZoneWithAiGateway(name string) *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Status: adminv1.ZoneStatus{
			Namespace: "default",
			GatewayRealm: &ctypes.ObjectRef{
				Name:      "gw-realm",
				Namespace: "default",
			},
			AiGatewayRealm: &ctypes.ObjectRef{
				Name:      "ai-gw-realm",
				Namespace: "default",
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

func makeReadyApplication(name, team, email, clientId string) *applicationv1.Application {
	app := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      team,
			TeamEmail: email,
		},
		Status: applicationv1.ApplicationStatus{
			ClientId: clientId,
		},
	}
	meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return app
}

var (
	subscriberZoneKey = k8stypes.NamespacedName{Name: "test-zone", Namespace: "default"}
	requestorAppKey   = k8stypes.NamespacedName{Name: "requestor-app", Namespace: "default"}
	providerAppKey    = k8stypes.NamespacedName{Name: "provider-app", Namespace: "default"}
)

var _ = Describe("McpSubscriptionHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		testScheme *runtime.Scheme
		h          *mcpsubscription.McpSubscriptionHandler
		obj        *agenticv1.McpSubscription
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		testScheme = buildScheme()
		h = &mcpsubscription.McpSubscriptionHandler{}
		obj = newMcpSubscription("test-sub", "/mcp/weather/v1", "test-zone")
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

	mockGetZone := func(key k8stypes.NamespacedName, zone *adminv1.Zone) {
		fakeClient.EXPECT().
			Get(ctx, key, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil).Once()
	}

	mockGetZoneError := func(key k8stypes.NamespacedName, err error) {
		fakeClient.EXPECT().
			Get(ctx, key, mock.AnythingOfType("*v1.Zone")).
			Return(err).Once()
	}

	mockGetApplication := func(key k8stypes.NamespacedName, app *applicationv1.Application) {
		fakeClient.EXPECT().
			Get(ctx, key, mock.AnythingOfType("*v1.Application")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationv1.Application) = *app
			}).
			Return(nil).Once()
	}

	mockScheme := func() {
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	}

	mockApprovalBuilderGranted := func() {
		// 1. CreateOrUpdate ApprovalRequest
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Return(controllerutil.OperationResultCreated, nil).Once()

		// 2. Cleanup old ApprovalRequests
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil).Once()

		// 3. Get Approval — return a Granted Approval
		fakeClient.EXPECT().
			Get(ctx, mock.Anything, mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Spec.State = approvalv1.ApprovalStateGranted
			}).
			Return(nil).Once()
	}

	mockApprovalBuilderPending := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Return(controllerutil.OperationResultCreated, nil).Once()

		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil).Once()

		// Approval not found — results in Pending
		fakeClient.EXPECT().
			Get(ctx, mock.Anything, mock.AnythingOfType("*v1.Approval")).
			Return(apierrors.NewNotFound(schema.GroupResource{}, "")).Once()
	}

	mockApprovalBuilderDenied := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Return(controllerutil.OperationResultCreated, nil).Once()

		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil).Once()

		// Approval found with Rejected state
		fakeClient.EXPECT().
			Get(ctx, mock.Anything, mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Spec.State = approvalv1.ApprovalStateRejected
			}).
			Return(nil).Once()
	}

	mockCreateOrUpdateConsumeRoute := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ConsumeRoute"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	// setupPreApprovalMocks sets up mocks for McpServer + McpExposure lookup, zone,
	// and application lookups (everything before the approval step).
	setupPreApprovalMocks := func() {
		server := makeReadyMcpServer("/mcp/weather/v1")
		exposure := makeReadyMcpExposure("/mcp/weather/v1", "test-zone")
		zone := makeReadyZoneWithAiGateway("test-zone")
		requestorApp := makeReadyApplication("requestor-app", "requestor-team", "req@example.com", "req-client-id")
		providerApp := makeReadyApplication("provider-app", "provider-team", "prov@example.com", "prov-client-id")

		mockListMcpServers([]agenticv1.McpServer{server})
		mockListMcpExposures([]agenticv1.McpExposure{exposure})
		mockGetZone(subscriberZoneKey, zone)
		mockGetApplication(requestorAppKey, requestorApp)
		mockGetApplication(providerAppKey, providerApp)
		mockScheme()
	}

	// setupFullHappyPath sets up all mocks for a complete successful CreateOrUpdate.
	setupFullHappyPath := func() {
		setupPreApprovalMocks()
		mockApprovalBuilderGranted()
		mockCreateOrUpdateConsumeRoute(controllerutil.OperationResultCreated, nil)
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

		It("should set Blocked when no active McpExposure found", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("McpExposureNotFound"))
		})

		It("should return BlockedError when subscriber zone does not support AI Gateway", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			exposure := makeReadyMcpExposure("/mcp/weather/v1", "test-zone")

			// Zone without AI Gateway feature
			zone := &adminv1.Zone{
				ObjectMeta: metav1.ObjectMeta{Name: "test-zone", Namespace: "default"},
				Status: adminv1.ZoneStatus{
					Namespace: "default",
				},
			}
			meta.SetStatusCondition(&zone.Status.Conditions, metav1.Condition{
				Type:   condition.ConditionTypeReady,
				Status: metav1.ConditionTrue,
				Reason: "Ready",
			})

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{exposure})
			mockGetZone(subscriberZoneKey, zone)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(isBlockedError(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("AI Gateway feature"))
		})

		It("should return BlockedError when visibility constraints are violated", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")

			// Exposure with Zone visibility - only same-zone allowed
			exposure := makeReadyMcpExposure("/mcp/weather/v1", "provider-zone")
			exposure.Spec.Visibility = agenticv1.VisibilityZone

			zone := makeReadyZoneWithAiGateway("test-zone")

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{exposure})
			mockGetZone(subscriberZoneKey, zone)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(isBlockedError(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("visibility constraints"))
		})

		It("should return error when GetZone fails", func() {
			server := makeReadyMcpServer("/mcp/weather/v1")
			exposure := makeReadyMcpExposure("/mcp/weather/v1", "test-zone")

			mockListMcpServers([]agenticv1.McpServer{server})
			mockListMcpExposures([]agenticv1.McpExposure{exposure})
			mockGetZoneError(subscriberZoneKey, fmt.Errorf("zone lookup failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("zone"))
		})

		It("should set NotReady when approval is pending", func() {
			setupPreApprovalMocks()
			mockApprovalBuilderPending()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ApprovalPending"))
		})

		It("should set NotReady when approval is denied and cleanup ConsumeRoute", func() {
			setupPreApprovalMocks()
			mockApprovalBuilderDenied()

			// Cleanup call for denied approval
			fakeClient.EXPECT().
				Cleanup(ctx, mock.AnythingOfType("*v1.ConsumeRouteList"), mock.Anything).
				Return(0, nil).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ApprovalDenied"))
		})

		It("should create ConsumeRoute and set Ready when approval is granted", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.ConsumeRoute).ToNot(BeNil())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("McpSubscriptionProvisioned"))
		})

		It("should set NotReady when AllReady returns false", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(false).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.ConsumeRoute).ToNot(BeNil())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ChildResourcesNotReady"))
		})

		It("should return error when ConsumeRoute creation fails", func() {
			setupPreApprovalMocks()
			mockApprovalBuilderGranted()
			mockCreateOrUpdateConsumeRoute(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create ConsumeRoute"))
		})
	})

	Describe("Delete", func() {
		It("should cleanup ConsumeRoute owned by the subscription", func() {
			fakeClient.EXPECT().
				Cleanup(ctx, mock.AnythingOfType("*v1.ConsumeRouteList"), mock.Anything).
				Return(1, nil).Once()

			err := h.Delete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when Cleanup fails", func() {
			fakeClient.EXPECT().
				Cleanup(ctx, mock.AnythingOfType("*v1.ConsumeRouteList"), mock.Anything).
				Return(0, fmt.Errorf("cleanup failed")).Once()

			err := h.Delete(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to cleanup ConsumeRoute"))
		})
	})
})
