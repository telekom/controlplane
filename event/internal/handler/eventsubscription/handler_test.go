// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventsubscription_test

import (
	"context"
	"errors"
	"fmt"

	pkgerrors "github.com/pkg/errors"
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
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventsubscription"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func isBlockedError(err error) bool {
	var be ctrlerrors.BlockedError
	if errors.As(err, &be) && be.IsBlocked() {
		return true
	}
	cause := pkgerrors.Cause(err)
	if errors.As(cause, &be) && be.IsBlocked() {
		return true
	}
	return false
}

// buildScheme creates a runtime.Scheme with all types needed by the handler.
func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = adminv1.AddToScheme(s)
	_ = eventv1.AddToScheme(s)
	_ = applicationv1.AddToScheme(s)
	_ = approvalv1.AddToScheme(s)
	_ = pubsubv1.AddToScheme(s)
	return s
}

// --- Factory helpers ---

func newEventSubscription(name, eventType, zoneName string) *eventv1.EventSubscription {
	return &eventv1.EventSubscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "sub-uid-1234",
		},
		Spec: eventv1.EventSubscriptionSpec{
			EventType: eventType,
			Zone:      ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			Requestor: ctypes.TypedObjectRef{
				TypeMeta:  metav1.TypeMeta{Kind: "Application"},
				ObjectRef: ctypes.ObjectRef{Name: "requestor-app", Namespace: "default"},
			},
			Delivery: eventv1.Delivery{
				Type:    eventv1.DeliveryTypeCallback,
				Payload: eventv1.PayloadTypeData,
				// Callback is set per-test for callback scenarios
			},
		},
	}
}

func makeReadyEventType(eventType string) eventv1.EventType {
	et := eventv1.EventType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      eventv1.MakeEventTypeName(eventType),
			Namespace: "default",
		},
		Spec: eventv1.EventTypeSpec{
			Type:    eventType,
			Version: "1.0.0",
		},
		Status: eventv1.EventTypeStatus{
			Active: true,
		},
	}
	meta.SetStatusCondition(&et.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return et
}

func makeReadyEventExposure(eventType string) eventv1.EventExposure {
	exp := eventv1.EventExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exposure",
			Namespace: "default",
			UID:       "exposure-uid",
		},
		Spec: eventv1.EventExposureSpec{
			EventType:  eventType,
			Visibility: eventv1.VisibilityEnterprise,
			Zone:       ctypes.ObjectRef{Name: "expo-zone", Namespace: "default"},
			Provider:   ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "provider-app", Namespace: "default"}},
			Approval: eventv1.Approval{
				Strategy: eventv1.ApprovalStrategyAuto,
			},
		},
		Status: eventv1.EventExposureStatus{
			Active: true,
			Publisher: &ctypes.ObjectRef{
				Name:      "test-publisher",
				Namespace: "default",
			},
		},
	}
	meta.SetStatusCondition(&exp.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return exp
}

func makeReadyEventConfig(zoneName string, fullMesh bool, meshZones []string) eventv1.EventConfig {
	ec := eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zoneName + "-eventconfig",
			Namespace: "default",
		},
		Spec: eventv1.EventConfigSpec{
			Zone: ctypes.ObjectRef{Name: zoneName, Namespace: "default"},
			Mesh: eventv1.MeshConfig{
				FullMesh:  fullMesh,
				ZoneNames: meshZones,
			},
		},
		Status: eventv1.EventConfigStatus{
			ProxyCallbackURLs: map[string]string{},
		},
	}
	meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return ec
}

func makeReadyApplication(name, team, teamEmail, clientId string) *applicationv1.Application {
	app := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       k8stypes.UID(name + "-uid"),
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      team,
			TeamEmail: teamEmail,
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

func makeReadyZone(name, namespace string) *adminv1.Zone {
	zone := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: adminv1.ZoneSpec{
			Visibility: adminv1.ZoneVisibilityEnterprise,
		},
	}
	meta.SetStatusCondition(&zone.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return zone
}

var (
	testEventType   = "de.telekom.eni.quickstart.v1"
	requestorAppKey = k8stypes.NamespacedName{Name: "requestor-app", Namespace: "default"}
	providerAppKey  = k8stypes.NamespacedName{Name: "provider-app", Namespace: "default"}
)

var _ = Describe("EventSubscriptionHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		testScheme *runtime.Scheme
		h          *eventsubscription.EventSubscriptionHandler
		obj        *eventv1.EventSubscription
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		testScheme = buildScheme()
		h = &eventsubscription.EventSubscriptionHandler{}
		// Default: same-zone callback subscription
		obj = newEventSubscription("test-sub", testEventType, "expo-zone")
		obj.Spec.Delivery.Callback = "https://my-callback.example.com"
	})

	// --- mock helpers ---

	mockListEventTypes := func(items []eventv1.EventType) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventTypeList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventTypeList) = eventv1.EventTypeList{Items: items}
			}).
			Return(nil).Once()
	}

	mockListEventTypesError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventTypeList")).
			Return(err).Once()
	}

	mockListEventExposures := func(items []eventv1.EventExposure) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventExposureList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventExposureList) = eventv1.EventExposureList{Items: items}
			}).
			Return(nil).Once()
	}

	mockListEventExposuresError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventExposureList"), mock.Anything).
			Return(err).Once()
	}

	// mockListEventConfigs stubs c.List for EventConfigList the given number of times.
	mockListEventConfigs := func(items []eventv1.EventConfig, times int) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: items}
			}).
			Return(nil).Times(times)
	}

	mockListEventConfigsError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
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

	mockGetApplicationError := func(key k8stypes.NamespacedName, err error) {
		fakeClient.EXPECT().
			Get(ctx, key, mock.AnythingOfType("*v1.Application")).
			Return(err).Once()
	}

	mockGetZone := func(key k8stypes.NamespacedName, zone *adminv1.Zone) {
		fakeClient.EXPECT().
			Get(ctx, key, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil).Once()
	}

	mockScheme := func() {
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	}

	// mockApprovalBuilderGranted stubs the three client calls the approval builder makes,
	// resulting in an "Granted" result. The Approval object returned by Get has state Granted.
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

	mockApprovalBuilderRequestDenied := func() {
		// CreateOrUpdate returns an ApprovalRequest with State=Rejected
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, _ controllerutil.MutateFn) {
				req := obj.(*approvalv1.ApprovalRequest)
				req.Spec.State = approvalv1.ApprovalStateRejected
			}).
			Return(controllerutil.OperationResultNone, nil).Once()

		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil).Once()

		// Approval found with Granted state (but RequestDenied takes priority from request)
		fakeClient.EXPECT().
			Get(ctx, mock.Anything, mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Spec.State = approvalv1.ApprovalStateGranted
			}).
			Return(nil).Once()
	}

	mockCreateOrUpdateSubscriber := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	mockCreateOrUpdateSubscriberWithSubscriptionId := func(subscriptionId string, result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				subscriber := obj.(*pubsubv1.Subscriber)
				subscriber.Status.SubscriptionId = subscriptionId
			}).
			Return(result, err).Once()
	}

	// mockCleanupSubscribers stubs the Cleanup call for SubscriberList.
	mockCleanupSubscribers := func(deleted int, err error) {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Return(deleted, err).Once()
	}

	// setupUpToApproval sets up mocks through the approval step (for same-zone scenarios).
	setupUpToApproval := func(exposure eventv1.EventExposure) {
		et := makeReadyEventType(testEventType)
		expoConfig := makeReadyEventConfig("expo-zone", true, nil) // FullMesh=true supports any zone

		mockListEventTypes([]eventv1.EventType{et})
		mockListEventExposures([]eventv1.EventExposure{exposure})
		// Visibility validation requires Zone lookup for the subscription's zone
		mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))
		mockListEventConfigs([]eventv1.EventConfig{expoConfig}, 2) // exposure zone + subscription zone (same zone)

		requestorApp := makeReadyApplication("requestor-app", "requester-team", "req@example.com", "req-client-id")
		providerApp := makeReadyApplication("provider-app", "provider-team", "prov@example.com", "prov-client-id")
		mockGetApplication(requestorAppKey, requestorApp)
		mockGetApplication(providerAppKey, providerApp)
		mockScheme()
	}

	// setupFullHappyPath sets up all mocks for a complete successful CreateOrUpdate (same-zone).
	setupFullHappyPath := func() {
		exposure := makeReadyEventExposure(testEventType)
		setupUpToApproval(exposure)
		mockApprovalBuilderGranted()
		mockCreateOrUpdateSubscriber(controllerutil.OperationResultCreated, nil)
	}

	// =====================================================================
	// CreateOrUpdate tests
	// =====================================================================

	Describe("CreateOrUpdate", func() {
		It("should return error when FindActiveEventType fails", func() {
			mockListEventTypesError(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list EventTypes"))
		})

		It("should set NotReady when no active EventType found", func() {
			mockListEventTypes([]eventv1.EventType{})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("EventTypeNotFound"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should return error when FindEventExposures fails", func() {
			et := makeReadyEventType(testEventType)
			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposuresError(fmt.Errorf("list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list EventExposures"))
		})

		It("should set NotReady when no EventExposure found (empty list) and cleanup succeeds", func() {
			et := makeReadyEventType(testEventType)
			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockCleanupSubscribers(0, nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("EventExposureNotFound"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should return error when Subscriber cleanup fails (no exposures path)", func() {
			et := makeReadyEventType(testEventType)
			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockCleanupSubscribers(0, fmt.Errorf("cleanup failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to cleanup Subscriber"))
		})

		It("should set NotReady when exposure exists but is not active", func() {
			et := makeReadyEventType(testEventType)
			mockListEventTypes([]eventv1.EventType{et})

			// Inactive exposure
			exp := makeReadyEventExposure(testEventType)
			exp.Status.Active = false
			mockListEventExposures([]eventv1.EventExposure{exp})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("EventExposureNotFound"))
		})

		It("should return error when GetEventConfigForZone fails for exposure zone", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))
			mockListEventConfigsError(fmt.Errorf("config list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get EventConfig for exposure zone"))
		})

		It("should set NotReady when subscription zone is not supported", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			// Subscription is in a different zone
			obj.Spec.Zone.Name = "other-zone"

			// EventConfig with FullMesh=false and no mesh zones → doesn't support "other-zone"
			expoConfig := makeReadyEventConfig("expo-zone", false, nil)
			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))
			mockListEventConfigs([]eventv1.EventConfig{expoConfig}, 1)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ZoneNotSupported"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should return error when GetEventConfigForZone fails for subscription zone", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			obj.Spec.Zone.Name = "sub-zone"

			// Exposure zone config supports sub-zone via mesh
			expoConfig := makeReadyEventConfig("expo-zone", false, []string{"sub-zone"})

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))

			// First EventConfigList call (exposure zone) succeeds
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{expoConfig}}
				}).
				Return(nil).Once()

			// Second EventConfigList call (subscription zone) fails
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Return(fmt.Errorf("sub-zone config list failed")).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get EventConfig for subscription zone"))
		})

		It("should return blocked error when cross-zone callback proxy URL is missing", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			obj.Spec.Zone.Name = "sub-zone"
			obj.Spec.Delivery.Callback = "https://my-callback.example.com"

			expoConfig := makeReadyEventConfig("expo-zone", true, nil)
			subConfig := makeReadyEventConfig("sub-zone", true, nil)
			// No ProxyCallbackURLs → missing proxy URL for "expo-zone"

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{expoConfig}}
				}).
				Return(nil).Once()

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{subConfig}}
				}).
				Return(nil).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(isBlockedError(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("no proxy callback URL"))
		})

		It("should update callback URL in cross-zone callback proxy scenario", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			obj.Spec.Zone.Name = "sub-zone"
			obj.Spec.Delivery.Callback = "https://my-callback.example.com"

			expoConfig := makeReadyEventConfig("expo-zone", true, nil)
			subConfig := makeReadyEventConfig("sub-zone", true, nil)
			subConfig.Status.ProxyCallbackURLs = map[string]string{
				"expo-zone": "https://proxy-callback.example.com/expo-zone",
			}

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{expoConfig}}
				}).
				Return(nil).Once()

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{subConfig}}
				}).
				Return(nil).Once()

			requestorApp := makeReadyApplication("requestor-app", "requester-team", "req@example.com", "req-client-id")
			providerApp := makeReadyApplication("provider-app", "provider-team", "prov@example.com", "prov-client-id")
			mockGetApplication(requestorAppKey, requestorApp)
			mockGetApplication(providerAppKey, providerApp)
			mockScheme()

			mockApprovalBuilderGranted()
			mockCreateOrUpdateSubscriber(controllerutil.OperationResultCreated, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			// Verify callback URL was updated
			Expect(obj.Spec.Delivery.Callback).To(ContainSubstring("https://proxy-callback.example.com/expo-zone"))
			Expect(obj.Spec.Delivery.Callback).To(ContainSubstring("callback=https://my-callback.example.com"))
		})

		It("should not modify callback URL for SSE delivery in cross-zone scenario", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			exposure.Status.SseURLs = map[string]string{"sub-zone": "https://sse.example.com/sub-zone"}
			obj.Spec.Zone.Name = "sub-zone"
			obj.Spec.Delivery.Type = eventv1.DeliveryTypeServerSentEvent
			obj.Spec.Delivery.Callback = "" // SSE has no callback

			expoConfig := makeReadyEventConfig("expo-zone", true, nil)
			subConfig := makeReadyEventConfig("sub-zone", true, nil)

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{expoConfig}}
				}).
				Return(nil).Once()

			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: []eventv1.EventConfig{subConfig}}
				}).
				Return(nil).Once()

			requestorApp := makeReadyApplication("requestor-app", "requester-team", "req@example.com", "req-client-id")
			providerApp := makeReadyApplication("provider-app", "provider-team", "prov@example.com", "prov-client-id")
			mockGetApplication(requestorAppKey, requestorApp)
			mockGetApplication(providerAppKey, providerApp)
			mockScheme()

			mockApprovalBuilderGranted()
			mockCreateOrUpdateSubscriberWithSubscriptionId("sub-123", controllerutil.OperationResultCreated, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Spec.Delivery.Callback).To(BeEmpty())
		})

		It("should set NotReady when requestor kind is not Application", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			obj.Spec.Requestor.Kind = "ServiceAccount" // unsupported

			expoConfig := makeReadyEventConfig("expo-zone", true, nil)

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))
			mockListEventConfigs([]eventv1.EventConfig{expoConfig}, 2) // exposure zone + subscription zone (same)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("InvalidRequestor"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should return error when GetApplication for requestor fails", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			expoConfig := makeReadyEventConfig("expo-zone", true, nil)

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))
			mockListEventConfigs([]eventv1.EventConfig{expoConfig}, 2)
			mockGetApplicationError(requestorAppKey, fmt.Errorf("not found"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should return error when GetApplication for provider fails", func() {
			et := makeReadyEventType(testEventType)
			exposure := makeReadyEventExposure(testEventType)
			expoConfig := makeReadyEventConfig("expo-zone", true, nil)

			requestorApp := makeReadyApplication("requestor-app", "requester-team", "req@example.com", "req-client-id")

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{exposure})
			mockGetZone(obj.Spec.Zone.K8s(), makeReadyZone(obj.Spec.Zone.Name, obj.Spec.Zone.Namespace))
			mockListEventConfigs([]eventv1.EventConfig{expoConfig}, 2)
			mockGetApplication(requestorAppKey, requestorApp)
			mockGetApplicationError(providerAppKey, fmt.Errorf("provider not found"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to get application from EventExposure provider"))
		})

		It("should set NotReady when approval is pending", func() {
			exposure := makeReadyEventExposure(testEventType)
			setupUpToApproval(exposure)
			mockApprovalBuilderPending()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ApprovalPending"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should set NotReady and cleanup when approval is denied", func() {
			exposure := makeReadyEventExposure(testEventType)
			setupUpToApproval(exposure)
			mockApprovalBuilderDenied()

			// Cleanup Subscriber on denial
			mockCleanupSubscribers(1, nil)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ApprovalDenied"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})

		It("should set NotReady when approval request is denied", func() {
			exposure := makeReadyEventExposure(testEventType)
			setupUpToApproval(exposure)
			mockApprovalBuilderRequestDenied()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ApprovalRequestDenied"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})

		It("should set NotReady when exposure has no Publisher reference yet", func() {
			exposure := makeReadyEventExposure(testEventType)
			exposure.Status.Publisher = nil // Publisher not yet created
			setupUpToApproval(exposure)
			mockApprovalBuilderGranted()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("PublisherNotReady"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Blocked"))
		})

		It("should return error when createSubscriber fails", func() {
			exposure := makeReadyEventExposure(testEventType)
			setupUpToApproval(exposure)
			mockApprovalBuilderGranted()
			mockCreateOrUpdateSubscriber(controllerutil.OperationResultNone, fmt.Errorf("subscriber create failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create Subscriber"))
		})

		It("should set NotReady when child resources are not ready", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(false).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ChildResourcesNotReady"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(processingCond.Reason).To(Equal("ChildResourcesNotReady"))
		})

		It("should set Ready when all provisioning succeeds", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("EventSubscriptionProvisioned"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))

			// Verify status refs are populated
			Expect(obj.Status.Subscriber).ToNot(BeNil())
			Expect(obj.Status.ApprovalRequest).ToNot(BeNil())
			Expect(obj.Status.Approval).ToNot(BeNil())
		})

		It("should copy SubscriptionId from Subscriber status to EventSubscription status", func() {
			exposure := makeReadyEventExposure(testEventType)
			setupUpToApproval(exposure)
			mockApprovalBuilderGranted()
			mockCreateOrUpdateSubscriberWithSubscriptionId("sub-123", controllerutil.OperationResultCreated, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.SubscriptionId).To(Equal("sub-123"))
		})

		It("should not set SubscriptionId when Subscriber has empty SubscriptionId", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.SubscriptionId).To(BeEmpty())
		})

		It("should return error when approval cleanup of subscribers fails on denial", func() {
			exposure := makeReadyEventExposure(testEventType)
			setupUpToApproval(exposure)
			mockApprovalBuilderDenied()
			mockCleanupSubscribers(0, fmt.Errorf("cleanup failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to cleanup Subscriber"))
		})
	})

	// =====================================================================
	// Delete tests
	// =====================================================================

	Describe("Delete", func() {
		It("should return nil (no-op)", func() {
			err := h.Delete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	// =====================================================================
	// mapDelivery / mapTrigger (tested indirectly via CreateOrUpdate)
	// =====================================================================

	Describe("CreateOrUpdate with trigger and delivery options", func() {
		It("should successfully provision with SSE delivery and triggers", func() {
			obj.Spec.Delivery = eventv1.Delivery{
				Type:    eventv1.DeliveryTypeServerSentEvent,
				Payload: eventv1.PayloadTypeDataRef,
			}
			obj.Spec.Trigger = &eventv1.EventTrigger{
				ResponseFilter: &eventv1.ResponseFilter{
					Paths: []string{"$.data.name"},
					Mode:  eventv1.ResponseFilterModeInclude,
				},
				SelectionFilter: &eventv1.SelectionFilter{
					Attributes: map[string]string{"source": "test"},
				},
			}
			obj.Spec.Scopes = []string{"scope-a", "scope-b"}

			// Build exposure with scopes that match the subscription's requested scopes
			exposure := makeReadyEventExposure(testEventType)
			exposure.Status.SseURLs = map[string]string{"expo-zone": "https://sse.example.com/expo-zone"}
			exposure.Spec.Scopes = []eventv1.EventScope{
				{Name: "scope-a", Trigger: eventv1.EventTrigger{}},
				{Name: "scope-b", Trigger: eventv1.EventTrigger{}},
			}
			setupUpToApproval(exposure)
			mockApprovalBuilderGranted()
			mockCreateOrUpdateSubscriberWithSubscriptionId("sub-456", controllerutil.OperationResultCreated, nil)
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
