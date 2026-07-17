// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"
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
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// --- Test constants ---

const (
	listenerName       = "test-listener"
	listenerNamespace  = "team-ns"
	consumerAppName    = "consumer-app"
	providerAppName    = "provider-app"
	spectreAppName     = "sa-consumer-app"
	consumerTeam       = "team-alpha"
	providerTeam       = "team-beta"
	consumerEmail      = "alpha@test.com"
	providerEmail      = "beta@test.com"
	consumerClientId   = "team-alpha--consumer-app"
	providerClientId   = "team-beta--provider-app"
	listenerZoneName   = "aws"
	listenerZoneNs     = "env-ns"
	listenerZoneStatus = "env-ns--aws"
	testApiBasePath    = "/api/v1/orders"
	testCallbackURL    = "https://callback.gateway.example.com/callback"
	testAppId          = "consumer-app"
)

// --- Test fixtures ---

func newListener() *spectrev1.Listener {
	return &spectrev1.Listener{
		ObjectMeta: metav1.ObjectMeta{
			Name:      listenerName,
			Namespace: listenerNamespace,
			UID:       "listener-uid-001",
		},
		Spec: spectrev1.ListenerSpec{
			Consumer: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
				ObjectRef: ctypes.ObjectRef{
					Name:      consumerAppName,
					Namespace: listenerNamespace,
				},
			},
			Provider: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
				ObjectRef: ctypes.ObjectRef{
					Name:      providerAppName,
					Namespace: listenerNamespace,
				},
			},
			ApiListener: &spectrev1.ApiListener{
				ApiBasePath: testApiBasePath,
			},
		},
	}
}

func makeConsumerApp() *applicationv1.Application {
	app := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consumerAppName,
			Namespace: listenerNamespace,
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      consumerTeam,
			TeamEmail: consumerEmail,
			Zone:      ctypes.ObjectRef{Name: listenerZoneName, Namespace: listenerZoneNs},
		},
		Status: applicationv1.ApplicationStatus{
			ClientId: consumerClientId,
		},
	}
	meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return app
}

func makeProviderApp() *applicationv1.Application {
	app := &applicationv1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      providerAppName,
			Namespace: listenerNamespace,
		},
		Spec: applicationv1.ApplicationSpec{
			Team:      providerTeam,
			TeamEmail: providerEmail,
			Zone:      ctypes.ObjectRef{Name: listenerZoneName, Namespace: listenerZoneNs},
		},
		Status: applicationv1.ApplicationStatus{
			ClientId: providerClientId,
		},
	}
	meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return app
}

func makeSpectreApp() spectrev1.SpectreApplication {
	sa := spectrev1.SpectreApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spectreAppName,
			Namespace: listenerNamespace,
		},
		Spec: spectrev1.SpectreApplicationSpec{
			Application: ctypes.TypedObjectRef{
				TypeMeta: metav1.TypeMeta{Kind: "Application", APIVersion: "application.cp.ei.telekom.de/v1"},
				ObjectRef: ctypes.ObjectRef{
					Name:      consumerAppName,
					Namespace: listenerNamespace,
				},
			},
		},
		Status: spectrev1.SpectreApplicationStatus{
			Id: testAppId,
		},
	}
	return sa
}

func makeListenerZone() *adminv1.Zone {
	z := &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      listenerZoneName,
			Namespace: listenerZoneNs,
		},
		Status: adminv1.ZoneStatus{
			Namespace: listenerZoneStatus,
			Gateway: &ctypes.ObjectRef{
				Name:      "gateway-aws",
				Namespace: listenerZoneStatus,
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

func makeListenerEventConfig() eventv1.EventConfig {
	ec := eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ec-aws",
			Namespace: listenerZoneStatus,
		},
		Spec: eventv1.EventConfigSpec{
			Zone: ctypes.ObjectRef{Name: listenerZoneName, Namespace: listenerZoneNs},
			Local: &eventv1.LocalBackend{
				Admin:              eventv1.AdminConfig{Url: "http://admin.local"},
				ServerSendEventUrl: "https://horizon-sse.internal:443/api/v1/sse",
				PublishEventUrl:    "http://publish.local",
			},
		},
		Status: eventv1.EventConfigStatus{
			CallbackURL: testCallbackURL,
		},
	}
	meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return ec
}

func makeListenerEventStore() pubsubv1.EventStore {
	return pubsubv1.EventStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eventstore-aws",
			Namespace: listenerZoneStatus,
		},
		Spec: pubsubv1.EventStoreSpec{
			Url:          "http://admin.local",
			TokenUrl:     "http://token.local",
			ClientId:     "client-id",
			ClientSecret: "client-secret",
		},
	}
}

// --- Tests ---

var _ = Describe("ListenerHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *handler.ListenerHandler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &handler.ListenerHandler{}

		scheme = runtime.NewScheme()
		_ = spectrev1.AddToScheme(scheme)
		_ = approvalv1.AddToScheme(scheme)
		_ = applicationv1.AddToScheme(scheme)
		fakeClient.EXPECT().Scheme().Return(scheme).Maybe()
	})

	// --- Mock helpers ---

	mockGetConsumerApp := func(app *applicationv1.Application) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: consumerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationv1.Application) = *app
			}).
			Return(nil).Once()
	}

	mockGetProviderApp := func(app *applicationv1.Application) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: providerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*applicationv1.Application) = *app
			}).
			Return(nil).Once()
	}

	mockListSpectreApps := func(items []spectrev1.SpectreApplication) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SpectreApplicationList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*spectrev1.SpectreApplicationList) = spectrev1.SpectreApplicationList{Items: items}
			}).
			Return(nil).Once()
	}

	mockGetZone := func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: listenerZoneName, Namespace: listenerZoneNs}, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *makeListenerZone()
			}).
			Return(nil)
	}

	mockListEventConfigs := func(items []eventv1.EventConfig) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{Items: items}
			}).
			Return(nil)
	}

	mockListEventStores := func(items []pubsubv1.EventStore) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventStoreList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*pubsubv1.EventStoreList) = pubsubv1.EventStoreList{Items: items}
			}).
			Return(nil).Once()
	}

	// mockApprovalGranted sets up the approval builder mock chain for an auto-granted approval.
	mockApprovalGranted := func() {
		// The ApprovalBuilder calls CreateOrUpdate (for ApprovalRequest), Cleanup, then Get (for Approval).
		// For auto-approved (same team), the builder sets state to Granted internally.
		// We mock two approval builds (provider + consumer).
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				req := obj.(*approvalv1.ApprovalRequest)
				_ = mutate()
				// Simulate auto-approval: strategy=Auto means state=Granted
				if req.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
					req.Spec.State = approvalv1.ApprovalStateGranted
				}
			}).
			Return(controllerutil.OperationResultCreated, nil)

		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil)

		// Get Approval — return auto-granted Approval
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Name = key.Name
				approval.Namespace = key.Namespace
				approval.Spec.State = approvalv1.ApprovalStateGranted
			}).
			Return(nil)
	}

	// mockApprovalPending sets up mock chain where approval is pending (not yet granted).
	mockApprovalPending := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil)

		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil)

		// Get Approval — return NotFound (pending)
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Return(errors.NewNotFound(schema.GroupResource{Group: "approval.cp.ei.telekom.de", Resource: "approvals"}, ""))
	}

	mockListRoutes := func() {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*gatewayv1.RouteList) = gatewayv1.RouteList{
					Items: []gatewayv1.Route{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "route-orders",
								Namespace: listenerZoneStatus,
							},
							Spec: gatewayv1.RouteSpec{
								Paths: []string{testApiBasePath},
							},
						},
					},
				}
			}).
			Return(nil).Once()
	}

	mockCreateOrUpdatePublisher := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
	}

	mockCreateOrUpdateRouteListener := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()
	}

	mockCreateOrUpdateSubscriber := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil)
	}

	setupFullHappyPath := func() *spectrev1.Listener {
		listener := newListener()
		mockGetConsumerApp(makeConsumerApp())
		mockGetProviderApp(makeProviderApp())
		mockListSpectreApps([]spectrev1.SpectreApplication{makeSpectreApp()})
		mockGetZone()
		mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
		mockListEventStores([]pubsubv1.EventStore{makeListenerEventStore()})
		mockApprovalGranted()
		mockCreateOrUpdatePublisher()
		mockListRoutes()
		mockCreateOrUpdateRouteListener()
		mockCreateOrUpdateSubscriber()
		return listener
	}

	Describe("CreateOrUpdate", func() {
		Context("when approvals are pending", func() {
			It("should set Blocked condition and NOT create downstream resources", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockListSpectreApps([]spectrev1.SpectreApplication{makeSpectreApp()})
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockListEventStores([]pubsubv1.EventStore{makeListenerEventStore()})
				mockApprovalPending()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// Should have Blocked condition
				procCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(procCond).ToNot(BeNil())
				Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))

				// Should NOT have RouteListener
				Expect(listener.Status.RouteListener).To(BeNil())
				// Should NOT have EventSubscriptions
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})
		})

		Context("when approvals are granted", func() {
			It("should create RouteListener with correct fields", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockListSpectreApps([]spectrev1.SpectreApplication{makeSpectreApp()})
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockListEventStores([]pubsubv1.EventStore{makeListenerEventStore()})
				mockApprovalGranted()
				mockCreateOrUpdatePublisher()
				mockListRoutes()

				// Capture RouteListener
				var capturedRL *gatewayv1.RouteListener
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedRL = obj.(*gatewayv1.RouteListener)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockCreateOrUpdateSubscriber()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedRL).ToNot(BeNil())
				Expect(capturedRL.Namespace).To(Equal(listenerZoneStatus))
				Expect(capturedRL.Spec.Consumer).To(Equal(consumerClientId))
				Expect(capturedRL.Spec.ServiceOwner).To(Equal(providerClientId))
				Expect(capturedRL.Spec.Issue).To(Equal(testApiBasePath))
				Expect(capturedRL.Spec.Zone.Name).To(Equal(listenerZoneName))
				// Verify Route reference points to the resolved Route, not to RouteListener itself
				Expect(capturedRL.Spec.Route.Name).To(Equal("route-orders"))
				Expect(capturedRL.Spec.Route.Namespace).To(Equal(listenerZoneStatus))
			})

			It("should create generic Publisher with correct event type", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockListSpectreApps([]spectrev1.SpectreApplication{makeSpectreApp()})
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockListEventStores([]pubsubv1.EventStore{makeListenerEventStore()})
				mockApprovalGranted()

				// Capture Publisher
				var capturedPub *pubsubv1.Publisher
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedPub = obj.(*pubsubv1.Publisher)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockListRoutes()
				mockCreateOrUpdateRouteListener()
				mockCreateOrUpdateSubscriber()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedPub).ToNot(BeNil())
				Expect(capturedPub.Spec.EventType).To(Equal("de.telekom.ei.listener"))
				Expect(capturedPub.Spec.PublisherId).To(Equal("gateway"))
				Expect(capturedPub.Spec.EventStore.Name).To(Equal("eventstore-aws"))
			})

			It("should create two bridge Subscribers with correct selection filters", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockListSpectreApps([]spectrev1.SpectreApplication{makeSpectreApp()})
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockListEventStores([]pubsubv1.EventStore{makeListenerEventStore()})
				mockApprovalGranted()
				mockCreateOrUpdatePublisher()
				mockListRoutes()
				mockCreateOrUpdateRouteListener()

				// Capture both Subscribers
				var capturedSubs []*pubsubv1.Subscriber
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						sub := obj.(*pubsubv1.Subscriber)
						capturedSubs = append(capturedSubs, sub.DeepCopy())
					}).
					Return(controllerutil.OperationResultCreated, nil).Times(2)

				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedSubs).To(HaveLen(2))

				// First subscriber (rq)
				rqSub := capturedSubs[0]
				Expect(rqSub.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeCallback))
				Expect(rqSub.Spec.Delivery.Callback).To(ContainSubstring(testCallbackURL))
				Expect(rqSub.Spec.Delivery.Callback).To(ContainSubstring("listener=" + testAppId))
				Expect(rqSub.Spec.Trigger).ToNot(BeNil())
				Expect(rqSub.Spec.Trigger.SelectionFilter).ToNot(BeNil())
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["issue"]).To(Equal(testApiBasePath))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["consumer"]).To(Equal(consumerClientId))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["provider"]).To(Equal(providerClientId))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("REQUEST"))

				// Second subscriber (rp)
				rpSub := capturedSubs[1]
				Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("RESPONSE"))
			})

			It("should set Ready condition when all children are ready", func() {
				listener := setupFullHappyPath()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCond.Reason).To(Equal(condition.ReasonProvisioned))
			})

			It("should set NotReady when AllReady returns false", func() {
				listener := setupFullHappyPath()
				fakeClient.EXPECT().AllReady().Return(false).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCond.Reason).To(Equal(condition.ReasonSubResourceNotReady))
			})
		})

		Context("error handling", func() {
			It("should return error when consumer Application is not found", func() {
				listener := newListener()
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: consumerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
					Return(fmt.Errorf("not found")).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer Application"))
			})
		})
	})

	Describe("Delete", func() {
		Context("when this is the last Listener", func() {
			It("should delete the generic Publisher", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// List Listeners — only this one (being deleted)
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*spectrev1.ListenerList) = spectrev1.ListenerList{
							Items: []spectrev1.Listener{*listener},
						}
					}).
					Return(nil).Once()

				// Expect Delete call for the Publisher
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when other Listeners still exist", func() {
			It("should NOT delete the generic Publisher", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				otherListener := newListener()
				otherListener.Name = "other-listener"

				// List Listeners — this one + another one
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*spectrev1.ListenerList) = spectrev1.ListenerList{
							Items: []spectrev1.Listener{*listener, *otherListener},
						}
					}).
					Return(nil).Once()

				// No Delete call expected
				err := h.Delete(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
