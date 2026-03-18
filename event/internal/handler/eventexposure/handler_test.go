// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventexposure"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func isBlockedError(err error) bool {
	for e := err; e != nil; e = pkgerrors.Unwrap(e) {
		if be, ok := e.(ctrlerrors.BlockedError); ok && be.IsBlocked() {
			return true
		}
	}
	cause := pkgerrors.Cause(err)
	if be, ok := cause.(ctrlerrors.BlockedError); ok && be.IsBlocked() {
		return true
	}
	return false
}

func newEventExposure(name, eventType string) *eventv1.EventExposure {
	return &eventv1.EventExposure{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: eventv1.EventExposureSpec{
			EventType:  eventType,
			Visibility: eventv1.VisibilityEnterprise,
			Zone:       ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
			Provider:   ctypes.TypedObjectRef{ObjectRef: ctypes.ObjectRef{Name: "test-app", Namespace: "default"}},
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

func makeReadyZone() *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-zone",
			Namespace: "default",
		},
		Status: adminv1.ZoneStatus{
			Namespace: "default",
			GatewayRealm: &ctypes.ObjectRef{
				Name:      "gw-realm",
				Namespace: "default",
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

func makeReadyEventConfig() eventv1.EventConfig {
	ec := eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-eventconfig",
			Namespace: "default",
		},
		Spec: eventv1.EventConfigSpec{
			Zone: ctypes.ObjectRef{Name: "test-zone", Namespace: "default"},
			Admin: eventv1.AdminConfig{
				Url: "https://admin.example.com",
				Client: eventv1.ClientConfig{
					Realm: ctypes.ObjectRef{Name: "test-realm", Namespace: "default"},
				},
			},
			ServerSendEventUrl: "https://sse.example.com",
			PublishEventUrl:    "http://publish.internal:8080/publish",
		},
		Status: eventv1.EventConfigStatus{
			EventStore: &ctypes.ObjectRef{
				Name:      "test-eventstore",
				Namespace: "default",
			},
			CallbackURL: "https://callback.example.com/test-zone/callback/v1",
		},
	}
	meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return ec
}

func makeReadyEventStore() *pubsubv1.EventStore {
	es := &pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-eventstore",
			Namespace: "default",
		},
		Spec: pubsubv1.EventStoreSpec{
			Url:          "https://eventstore.example.com",
			TokenUrl:     "https://token.example.com/token",
			ClientId:     "es-client-id",
			ClientSecret: "es-client-secret",
		},
	}
	meta.SetStatusCondition(&es.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return es
}

func makeReadyApplication() *applicationv1.Application {
	app := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Status: applicationv1.ApplicationStatus{
			ClientId:     "app-client-id",
			ClientSecret: "app-client-secret",
		},
	}
	meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return app
}

func makeReadyGatewayRealm() *gatewayv1.Realm {
	r := &gatewayv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-realm",
			Namespace: "default",
		},
		Spec: gatewayv1.RealmSpec{
			Urls:       []string{"https://gateway.example.com:443"},
			IssuerUrls: []string{"https://issuer.example.com"},
		},
	}
	meta.SetStatusCondition(&r.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return r
}

var (
	zoneKey       = k8stypes.NamespacedName{Name: "test-zone", Namespace: "default"}
	eventStoreKey = k8stypes.NamespacedName{Name: "test-eventstore", Namespace: "default"}
	appKey        = k8stypes.NamespacedName{Name: "test-app", Namespace: "default"}
	gwRealmKey    = k8stypes.NamespacedName{Name: "gw-realm", Namespace: "default"}
)

var _ = Describe("EventExposureHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *eventexposure.EventExposureHandler
		obj        *eventv1.EventExposure
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &eventexposure.EventExposureHandler{}
		obj = newEventExposure("test-exposure", "de.telekom.eni.quickstart.v1")
	})

	// --- mock helpers ---

	// mockListEventTypes sets up a mock for c.List on EventTypeList (no ListOption args).
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

	// mockListEventExposures sets up a mock for c.List on EventExposureList (with MatchingLabels).
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

	// mockGetZone sets up a mock for c.Get on the zone key (adminv1.Zone).
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

	// mockListEventConfigs sets up a mock for c.List on EventConfigList (with MatchingFields).
	// times controls how many times this mock is expected (GetEventConfigForZone + GetEventStoreForZone both call it).
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

	// mockGetEventStore sets up a mock for c.Get on the EventStore.
	mockGetEventStore := func(es *pubsubv1.EventStore) {
		fakeClient.EXPECT().
			Get(ctx, eventStoreKey, mock.AnythingOfType("*v1.EventStore")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.EventStore) = *es
			}).
			Return(nil).Once()
	}

	mockGetEventStoreError := func(err error) {
		fakeClient.EXPECT().
			Get(ctx, eventStoreKey, mock.AnythingOfType("*v1.EventStore")).
			Return(err).Once()
	}

	// mockGetApplication sets up a mock for c.Get on the Application.
	mockGetApplication := func(app *applicationv1.Application) {
		fakeClient.EXPECT().
			Get(ctx, appKey, mock.AnythingOfType("*v1.Application")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationv1.Application) = *app
			}).
			Return(nil).Once()
	}

	mockGetApplicationError := func(err error) {
		fakeClient.EXPECT().
			Get(ctx, appKey, mock.AnythingOfType("*v1.Application")).
			Return(err).Once()
	}

	// mockCreateOrUpdatePublisher sets up a mock for c.CreateOrUpdate on a Publisher.
	mockCreateOrUpdatePublisher := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	// mockListEventSubscriptions sets up a mock for c.List on EventSubscriptionList.
	mockListEventSubscriptions := func(items []eventv1.EventSubscription) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventSubscriptionList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventSubscriptionList) = eventv1.EventSubscriptionList{Items: items}
			}).
			Return(nil).Once()
	}

	mockListEventSubscriptionsError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventSubscriptionList"), mock.Anything).
			Return(err).Once()
	}

	// mockGetGatewayRealm sets up a mock for c.Get on the gateway Realm.
	mockGetGatewayRealm := func(realm *gatewayv1.Realm) {
		fakeClient.EXPECT().
			Get(ctx, gwRealmKey, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayv1.Realm) = *realm
			}).
			Return(nil).Once()
	}

	mockGetGatewayRealmError := func(err error) {
		fakeClient.EXPECT().
			Get(ctx, gwRealmKey, mock.AnythingOfType("*v1.Realm")).
			Return(err).Once()
	}

	// mockCreateOrUpdateRoute sets up a mock for c.CreateOrUpdate on a Route.
	mockCreateOrUpdateRoute := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	// mockCleanup sets up a mock for c.Cleanup on a RouteList.
	mockCleanup := func(deleted int, err error) {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(deleted, err).Once()
	}

	// setupFullHappyPath sets up all mocks needed for a full successful CreateOrUpdate
	// run with no cross-zone SSE subscriptions.
	setupFullHappyPath := func() {
		et := makeReadyEventType("de.telekom.eni.quickstart.v1")
		zone := makeReadyZone()
		ec := makeReadyEventConfig()
		es := makeReadyEventStore()
		app := makeReadyApplication()
		gwRealm := makeReadyGatewayRealm()

		mockListEventTypes([]eventv1.EventType{et})
		mockListEventExposures([]eventv1.EventExposure{})
		mockGetZone(zone)
		mockListEventConfigs([]eventv1.EventConfig{ec}, 2) // GetEventConfigForZone + GetEventStoreForZone
		mockGetEventStore(es)
		mockGetApplication(app)
		mockCreateOrUpdatePublisher(controllerutil.OperationResultCreated, nil)
		mockListEventSubscriptions([]eventv1.EventSubscription{}) // no cross-zone SSE
		mockGetGatewayRealm(gwRealm)                              // for CreateSSERoute
		mockCreateOrUpdateRoute(controllerutil.OperationResultCreated, nil)
		mockCleanup(0, nil)
	}

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
			Expect(obj.Status.Active).To(BeFalse())

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
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposuresError(fmt.Errorf("list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list EventExposures"))
		})

		It("should set NotReady when another active EventExposure already exists", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			mockListEventTypes([]eventv1.EventType{et})

			existingExposure := eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-exposure",
					Namespace: "default",
					UID:       "other-uid",
				},
				Spec: eventv1.EventExposureSpec{
					EventType: "de.telekom.eni.quickstart.v1",
				},
				Status: eventv1.EventExposureStatus{
					Active: true,
				},
			}
			meta.SetStatusCondition(&existingExposure.Status.Conditions, metav1.Condition{
				Type:   condition.ConditionTypeReady,
				Status: metav1.ConditionTrue,
				Reason: "Ready",
			})
			mockListEventExposures([]eventv1.EventExposure{existingExposure})

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			Expect(obj.Status.Active).To(BeFalse())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("EventExposureAlreadyExists"))
		})

		It("should return error when GetZone fails", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockGetZoneError(fmt.Errorf("zone fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("zone"))
		})

		It("should return error when GetEventConfigForZone fails", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			zone := makeReadyZone()

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockGetZone(zone)
			mockListEventConfigsError(fmt.Errorf("list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list EventConfigs"))
		})

		It("should return error when GetEventStoreForZone fails", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			zone := makeReadyZone()
			ec := makeReadyEventConfig()

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockGetZone(zone)
			// First call succeeds (GetEventConfigForZone in handler line 78)
			// Second call also succeeds (GetEventConfigForZone inside GetEventStoreForZone)
			mockListEventConfigs([]eventv1.EventConfig{ec}, 2)
			mockGetEventStoreError(fmt.Errorf("eventstore fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("eventstore fetch failed"))
		})

		It("should return error when GetApplication fails", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			zone := makeReadyZone()
			ec := makeReadyEventConfig()
			es := makeReadyEventStore()

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockGetZone(zone)
			mockListEventConfigs([]eventv1.EventConfig{ec}, 2)
			mockGetEventStore(es)
			mockGetApplicationError(fmt.Errorf("app fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get application"))
		})

		It("should return error when createPublisher fails", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			zone := makeReadyZone()
			ec := makeReadyEventConfig()
			es := makeReadyEventStore()
			app := makeReadyApplication()

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockGetZone(zone)
			mockListEventConfigs([]eventv1.EventConfig{ec}, 2)
			mockGetEventStore(es)
			mockGetApplication(app)
			mockCreateOrUpdatePublisher(controllerutil.OperationResultNone, fmt.Errorf("publisher create failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create Publisher"))
		})

		It("should return error when FindCrossZoneSSESubscriptionZones fails", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			zone := makeReadyZone()
			ec := makeReadyEventConfig()
			es := makeReadyEventStore()
			app := makeReadyApplication()

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockGetZone(zone)
			mockListEventConfigs([]eventv1.EventConfig{ec}, 2)
			mockGetEventStore(es)
			mockGetApplication(app)
			mockCreateOrUpdatePublisher(controllerutil.OperationResultCreated, nil)
			mockListEventSubscriptionsError(fmt.Errorf("subscription list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to find cross-zone SSE subscriptions"))
		})

		It("should return error when CreateSSERoute fails", func() {
			et := makeReadyEventType("de.telekom.eni.quickstart.v1")
			zone := makeReadyZone()
			ec := makeReadyEventConfig()
			es := makeReadyEventStore()
			app := makeReadyApplication()

			mockListEventTypes([]eventv1.EventType{et})
			mockListEventExposures([]eventv1.EventExposure{})
			mockGetZone(zone)
			mockListEventConfigs([]eventv1.EventConfig{ec}, 2)
			mockGetEventStore(es)
			mockGetApplication(app)
			mockCreateOrUpdatePublisher(controllerutil.OperationResultCreated, nil)
			mockListEventSubscriptions([]eventv1.EventSubscription{}) // no cross-zone
			mockGetGatewayRealmError(fmt.Errorf("realm fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create SSE Route"))
		})

		It("should set NotReady when AllReady is false", func() {
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

		It("should set Ready condition when all children ready", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(true).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("EventExposureProvisioned"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})
	})

	Describe("Delete", func() {

		It("should return error when AnyOtherEventExposureExists fails", func() {
			mockListEventExposuresError(fmt.Errorf("list failed"))

			err := h.Delete(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check for other EventExposures"))
		})

		It("should skip cleanup when another EventExposure exists", func() {
			otherExposure := eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-exposure",
					Namespace: "default",
					UID:       "other-uid",
				},
				Spec: eventv1.EventExposureSpec{
					EventType: "de.telekom.eni.quickstart.v1",
				},
			}
			mockListEventExposures([]eventv1.EventExposure{otherExposure})

			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
			// No further mock calls expected (no Delete calls)
		})

		It("should delete Publisher and Routes when no other exposure exists", func() {
			obj.Status.Publisher = &ctypes.ObjectRef{Name: "test-publisher", Namespace: "default"}
			obj.Status.Route = &ctypes.ObjectRef{Name: "test-route", Namespace: "default"}
			obj.Status.ProxyRoutes = []ctypes.ObjectRef{
				{Name: "proxy-route-1", Namespace: "default"},
			}

			mockListEventExposures([]eventv1.EventExposure{})

			// Delete Publisher
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Publisher")).
				Return(nil).Once()

			// DeleteRouteIfExists for Route: Get then Delete
			routeKey := k8stypes.NamespacedName{Name: "test-route", Namespace: "default"}
			fakeClient.EXPECT().
				Get(ctx, routeKey, mock.AnythingOfType("*v1.Route")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					route := out.(*gatewayv1.Route)
					route.Name = "test-route"
					route.Namespace = "default"
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Route")).
				Return(nil).Once()

			// DeleteRouteIfExists for ProxyRoute: Get then Delete
			proxyRouteKey := k8stypes.NamespacedName{Name: "proxy-route-1", Namespace: "default"}
			fakeClient.EXPECT().
				Get(ctx, proxyRouteKey, mock.AnythingOfType("*v1.Route")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					route := out.(*gatewayv1.Route)
					route.Name = "proxy-route-1"
					route.Namespace = "default"
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Route")).
				Return(nil).Once()

			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should tolerate NotFound on Publisher delete", func() {
			obj.Status.Publisher = &ctypes.ObjectRef{Name: "test-publisher", Namespace: "default"}

			mockListEventExposures([]eventv1.EventExposure{})

			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Group: "pubsub.cp.ei.telekom.de", Resource: "publishers"},
				"test-publisher",
			)
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Publisher")).
				Return(notFoundErr).Once()

			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error when Publisher delete fails", func() {
			obj.Status.Publisher = &ctypes.ObjectRef{Name: "test-publisher", Namespace: "default"}

			mockListEventExposures([]eventv1.EventExposure{})

			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Publisher")).
				Return(fmt.Errorf("delete failed")).Once()

			err := h.Delete(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to delete Publisher"))
		})
	})
})
