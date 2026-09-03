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
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"
	"github.com/telekom/controlplane/spectre/internal/handler/util"

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
	testRealmName      = "test-realm"
	testRealmIssuer    = "https://iris.example.com/auth/realms/test"
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
			Application: ctypes.ObjectRef{
				Name:      spectreAppName,
				Namespace: listenerNamespace,
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
			UID:       "consumer-uid-001",
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
			UID:       "provider-uid-001",
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

func makeSpectreAppPtr() *spectrev1.SpectreApplication {
	sa := makeSpectreApp()
	return &sa
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
			IdentityRealm: &ctypes.ObjectRef{
				Name:      testRealmName,
				Namespace: listenerZoneNs,
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
			EventStore: &ctypes.ObjectRef{
				Name:      "eventstore-aws",
				Namespace: listenerZoneStatus,
			},
		},
	}
	meta.SetStatusCondition(&ec.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return ec
}

func makeListenerEventStore() *pubsubv1.EventStore {
	es := &pubsubv1.EventStore{
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
	meta.SetStatusCondition(&es.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return es
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

	mockGetSpectreApp := func(sa *spectrev1.SpectreApplication) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: spectreAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.SpectreApplication")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*spectrev1.SpectreApplication) = *sa
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

	mockGetRealm := func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: testRealmName, Namespace: listenerZoneNs}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Realm) = identityv1.Realm{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testRealmName,
						Namespace: listenerZoneNs,
					},
					Status: identityv1.RealmStatus{
						IssuerUrl: testRealmIssuer,
					},
				}
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

	mockGetEventStore := func(es *pubsubv1.EventStore) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: es.Name, Namespace: es.Namespace}, mock.AnythingOfType("*v1.EventStore")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*pubsubv1.EventStore) = *es
			}).
			Return(nil).Once()
	}

	// mockNoStaleChildren stubs the List calls for stale-child removal,
	// returning empty lists (no existing children to clean up).
	mockNoStaleChildren := func() {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
			}).
			Return(nil).Once()

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
			}).
			Return(nil).Once()
	}

	// mockApprovalGranted sets up the approval builder mock chain for an auto-granted approval.
	mockApprovalGranted := func() {
		// The ApprovalBuilder calls CreateOrUpdate (for ApprovalRequest), Cleanup, then Get (for Approval).
		// For auto-approved (same team), the builder sets state to Granted internally.
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

	// mockApprovalDenied sets up mock chain where approval is rejected.
	mockApprovalDenied := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil)

		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil)

		// Get Approval — return rejected
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Name = key.Name
				approval.Namespace = key.Namespace
				approval.Spec.State = approvalv1.ApprovalStateRejected
			}).
			Return(nil)
	}

	// mockApprovalRequestDenied sets up mock chain where ApprovalRequest itself is rejected.
	mockApprovalRequestDenied := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				req := obj.(*approvalv1.ApprovalRequest)
				_ = mutate()
				req.Spec.State = approvalv1.ApprovalStateRejected
			}).
			Return(controllerutil.OperationResultNone, nil)

		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(0, nil)

		// Get Approval — exists and not denied (so RequestDenied branch is reached)
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

	// mockListRoutes stubs the gateway Route lookup.
	mockListRoutes := func() {
		routeName := util.MakeRouteName(testApiBasePath)
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: routeName, Namespace: listenerZoneStatus},
				mock.AnythingOfType("*v1.Route")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, obj client.Object, _ ...client.GetOption) {
				*obj.(*gatewayv1.Route) = gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: listenerZoneStatus,
					},
					Spec: gatewayv1.RouteSpec{
						Paths: []string{"/gateway" + testApiBasePath},
					},
				}
			}).
			Return(nil).Once()
	}

	// mockPassThroughRoute stubs the gateway Route lookup returning a pass-through route.
	mockPassThroughRoute := func() {
		routeName := util.MakeRouteName(testApiBasePath)
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: routeName, Namespace: listenerZoneStatus},
				mock.AnythingOfType("*v1.Route")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, obj client.Object, _ ...client.GetOption) {
				*obj.(*gatewayv1.Route) = gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: listenerZoneStatus,
					},
					Spec: gatewayv1.RouteSpec{
						Paths:       []string{"/gateway" + testApiBasePath},
						PassThrough: true,
					},
				}
			}).
			Return(nil).Once()
	}

	// mockFailoverRoute stubs the gateway Route lookup returning a failover route.
	mockFailoverRoute := func() {
		routeName := util.MakeRouteName(testApiBasePath)
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: routeName, Namespace: listenerZoneStatus},
				mock.AnythingOfType("*v1.Route")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, obj client.Object, _ ...client.GetOption) {
				*obj.(*gatewayv1.Route) = gatewayv1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeName,
						Namespace: listenerZoneStatus,
					},
					Spec: gatewayv1.RouteSpec{
						Paths: []string{"/gateway" + testApiBasePath},
						Traffic: gatewayv1.Traffic{
							Failover: &gatewayv1.Failover{
								TargetZoneName: "other-zone",
								Targets: []gatewayv1.FailoverTarget{
									{ZoneName: "other-zone", Upstream: gatewayv1.Upstream{
										Scheme: "https", Hostname: "failover.example.com", Port: 443, Path: "/api",
									}},
								},
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

	// mockJanitorCleanup stubs the Cleanup calls that happen after provisioning
	// on the Granted path (for RouteListenerList and SubscriberList).
	mockJanitorCleanup := func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
			Return(0, nil).Once()
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Return(0, nil).Once()
	}

	setupFullHappyPath := func() *spectrev1.Listener {
		listener := newListener()
		mockGetConsumerApp(makeConsumerApp())
		mockGetProviderApp(makeProviderApp())
		mockGetSpectreApp(makeSpectreAppPtr())
		mockGetZone()
		mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
		mockGetEventStore(makeListenerEventStore())
		mockListRoutes()
		mockNoStaleChildren()
		mockApprovalGranted()
		mockCreateOrUpdatePublisher()
		mockGetRealm()
		mockCreateOrUpdateRouteListener()
		mockCreateOrUpdateSubscriber()
		mockJanitorCleanup()
		return listener
	}

	Describe("CreateOrUpdate", func() {
		Context("when the SpectreApplication has not resolved its application id", func() {
			It("should block and NOT create any RouteListener or Subscriber", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())

				sa := makeSpectreAppPtr()
				sa.Status.Id = ""
				mockGetSpectreApp(sa)

				err := h.CreateOrUpdate(ctx, listener)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("has not resolved its application id"))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})
		})

		Context("when approvals are pending", func() {
			It("should set Blocked condition and NOT create downstream resources", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalPending()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				procCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(procCond).ToNot(BeNil())
				Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))

				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})
		})

		Context("when approval is denied", func() {
			It("should delete all owner-labelled capture children and clear status", func() {
				listener := newListener()
				// Pre-populate status refs to verify they are cleared.
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				}

				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalDenied()

				// Denial cleanup: List + Delete for RouteListeners and Subscribers.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
							Items: []gatewayv1.RouteListener{
								{ObjectMeta: metav1.ObjectMeta{Name: "old-rl", Namespace: listenerZoneStatus}},
							},
						}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
					Return(nil).Once()

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{ObjectMeta: metav1.ObjectMeta{Name: "old-sub-rq", Namespace: listenerZoneStatus}},
							},
						}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// Status must be cleared.
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())

				// AccessDenied condition must be set.
				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
			})
		})

		Context("when ApprovalRequest is denied (RequestDenied)", func() {
			It("should not delete same-fingerprint active children", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalRequestDenied()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// AccessDenied condition must be set.
				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
			})
		})

		Context("when approvals are granted", func() {
			It("should create RouteListener with correct fields and fingerprint label", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalGranted()
				mockCreateOrUpdatePublisher()
				mockGetRealm()

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
				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedRL).ToNot(BeNil())
				Expect(capturedRL.Namespace).To(Equal(listenerZoneStatus))
				Expect(capturedRL.Spec.Consumer).To(Equal(consumerClientId))
				Expect(capturedRL.Spec.ServiceOwner).To(Equal(providerClientId))
				Expect(capturedRL.Spec.Issue).To(Equal(testApiBasePath))
				Expect(capturedRL.Spec.Zone.Name).To(Equal(listenerZoneName))
				Expect(capturedRL.Spec.Route.Name).To(Equal(util.MakeRouteName(testApiBasePath)))
				Expect(capturedRL.Spec.Route.Namespace).To(Equal(listenerZoneStatus))

				// Verify authorization fingerprint label is present.
				Expect(capturedRL.Labels).To(HaveKey(handler.AuthorizationFingerprintLabelKey))
				Expect(capturedRL.Labels[handler.AuthorizationFingerprintLabelKey]).ToNot(BeEmpty())
			})

			It("should create generic Publisher with correct event type", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalGranted()

				var capturedPub *pubsubv1.Publisher
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedPub = obj.(*pubsubv1.Publisher)
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()

				mockGetRealm()
				mockCreateOrUpdateRouteListener()
				mockCreateOrUpdateSubscriber()
				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedPub).ToNot(BeNil())
				Expect(capturedPub.Spec.EventType).To(Equal("de.telekom.ei.listener"))
				Expect(capturedPub.Spec.PublisherId).To(Equal("gateway"))
				Expect(capturedPub.Spec.EventStore.Name).To(Equal("eventstore-aws"))
			})

			It("should create two bridge Subscribers with correct selection filters and fingerprint", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalGranted()
				mockCreateOrUpdatePublisher()
				mockGetRealm()
				mockCreateOrUpdateRouteListener()

				var capturedSubs []*pubsubv1.Subscriber
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						sub := obj.(*pubsubv1.Subscriber)
						capturedSubs = append(capturedSubs, sub.DeepCopy())
					}).
					Return(controllerutil.OperationResultCreated, nil).Times(2)

				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedSubs).To(HaveLen(2))

				rqSub := capturedSubs[0]
				Expect(rqSub.Spec.Delivery.Type).To(Equal(pubsubv1.DeliveryTypeCallback))
				// The callback must route through the Gateway, not raw localhost.
				Expect(rqSub.Spec.Delivery.Callback).To(ContainSubstring(testCallbackURL))
				Expect(rqSub.Spec.Delivery.Callback).To(ContainSubstring("callback="))
				Expect(rqSub.Spec.Delivery.Callback).To(ContainSubstring(testAppId))
				Expect(rqSub.Spec.Trigger).ToNot(BeNil())
				Expect(rqSub.Spec.Trigger.SelectionFilter).ToNot(BeNil())
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["issue"]).To(Equal(testApiBasePath))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["consumer"]).To(Equal(consumerClientId))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["provider"]).To(Equal(providerClientId))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("REQUEST"))

				// Verify fingerprint label on both Subscribers.
				Expect(rqSub.Labels).To(HaveKey(handler.AuthorizationFingerprintLabelKey))

				rpSub := capturedSubs[1]
				Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("RESPONSE"))
				Expect(rpSub.Labels).To(HaveKey(handler.AuthorizationFingerprintLabelKey))
			})

			It("should set Ready condition when all children are ready", func() {
				listener := setupFullHappyPath()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCond.Reason).To(Equal(condition.ReasonProvisioned))
			})

			It("should set NotReady when a sub-resource was just created or updated", func() {
				listener := setupFullHappyPath()
				fakeClient.EXPECT().AnyChanged().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCond.Reason).To(Equal(condition.ReasonSubResourceNotReady))
			})

			It("should set NotReady when AllReady returns false", func() {
				listener := setupFullHappyPath()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(false).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCond.Reason).To(Equal(condition.ReasonSubResourceNotReady))
			})
		})

		Context("approval properties", func() {
			It("should expose consumer, provider, path, listener app, directions, delivery, and filter scope", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()

				// Capture the ApprovalRequest to inspect properties.
				var capturedReq *approvalv1.ApprovalRequest
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						req := obj.(*approvalv1.ApprovalRequest)
						_ = mutate()
						if req.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
							req.Spec.State = approvalv1.ApprovalStateGranted
						}
						capturedReq = req.DeepCopy()
					}).
					Return(controllerutil.OperationResultCreated, nil)

				fakeClient.EXPECT().
					Cleanup(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
					Return(0, nil)

				fakeClient.EXPECT().
					Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Name = key.Name
						approval.Namespace = key.Namespace
						approval.Spec.State = approvalv1.ApprovalStateGranted
					}).
					Return(nil)

				mockCreateOrUpdatePublisher()
				mockGetRealm()
				mockCreateOrUpdateRouteListener()
				mockCreateOrUpdateSubscriber()
				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedReq).ToNot(BeNil())
				props, propErr := capturedReq.Spec.Requester.GetProperties()
				Expect(propErr).ToNot(HaveOccurred())
				Expect(props).To(HaveKey("consumer"))
				Expect(props).To(HaveKey("provider"))
				Expect(props).To(HaveKey("apiBasePath"))
				Expect(props).To(HaveKey("listenerApplication"))
				Expect(props).To(HaveKey("captureRequest"))
				Expect(props).To(HaveKey("captureResponse"))
				Expect(props).To(HaveKey("deliveryType"))
				Expect(props).To(HaveKey("requestFilter"))
				Expect(props).To(HaveKey("responseFilter"))

				// Requester and Decider ApplicationRefs must be set.
				Expect(capturedReq.Spec.Requester.ApplicationRef).ToNot(BeNil())
				Expect(capturedReq.Spec.Requester.ApplicationRef.Name).To(Equal(consumerAppName))
				Expect(capturedReq.Spec.Decider.ApplicationRef).ToNot(BeNil())
				Expect(capturedReq.Spec.Decider.ApplicationRef.Name).To(Equal(providerAppName))
			})
		})

		Context("provider changes (stale children)", func() {
			It("should delete old-fingerprint children before evaluating the replacement grant", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()

				// Simulate existing children with a different fingerprint (stale).
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
							Items: []gatewayv1.RouteListener{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name:      "stale-rl",
										Namespace: listenerZoneStatus,
										Labels: map[string]string{
											handler.AuthorizationFingerprintLabelKey: "old-fingerprint",
										},
									},
								},
							},
						}
					}).
					Return(nil).Once()

				// Expect stale RouteListener deletion.
				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetName() == "stale-rl"
					}), mock.Anything).
					Return(nil).Once()

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name:      "stale-sub",
										Namespace: listenerZoneStatus,
										Labels: map[string]string{
											handler.AuthorizationFingerprintLabelKey: "old-fingerprint",
										},
									},
								},
							},
						}
					}).
					Return(nil).Once()

				// Expect stale Subscriber deletion.
				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetName() == "stale-sub"
					}), mock.Anything).
					Return(nil).Once()

				// After stale cleanup, proceed with approval (pending to stop here).
				mockApprovalPending()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// Confirm blocked condition (pending).
				procCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(procCond).ToNot(BeNil())
				Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))
			})
		})

		Context("legacy children without a fingerprint", func() {
			It("should remove unlabelled legacy children (fail-closed migration)", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()

				// Simulate existing children WITHOUT fingerprint label (pre-migration).
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
							Items: []gatewayv1.RouteListener{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name:      "legacy-rl",
										Namespace: listenerZoneStatus,
										Labels:    map[string]string{},
									},
								},
							},
						}
					}).
					Return(nil).Once()

				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetName() == "legacy-rl"
					}), mock.Anything).
					Return(nil).Once()

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name:      "legacy-sub",
										Namespace: listenerZoneStatus,
										Labels:    map[string]string{},
									},
								},
							},
						}
					}).
					Return(nil).Once()

				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetName() == "legacy-sub"
					}), mock.Anything).
					Return(nil).Once()

				// After stale cleanup, approval proceeds (pending).
				mockApprovalPending()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("unsupported route modes", func() {
			It("should block with pass-through route and NOT create ApprovalRequest or children", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockPassThroughRoute()

				// deleteAllOwnedChildren: no existing children.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pass-through"))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})

			It("should block with failover route and NOT create ApprovalRequest or children", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockFailoverRoute()

				// deleteAllOwnedChildren: no existing children.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failover"))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})

			It("should remove existing children when Route transitions to pass-through", func() {
				listener := newListener()
				// Pre-populate status refs to verify they are cleared.
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				}

				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockPassThroughRoute()

				// deleteAllOwnedChildren: existing children returned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
							Items: []gatewayv1.RouteListener{
								{ObjectMeta: metav1.ObjectMeta{Name: "old-rl", Namespace: listenerZoneStatus}},
							},
						}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
					Return(nil).Once()

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{ObjectMeta: metav1.ObjectMeta{Name: "old-sub-rq", Namespace: listenerZoneStatus}},
							},
						}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pass-through"))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
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

				fakeClient.EXPECT().
					Cleanup(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Return(0, nil).Maybe()
				fakeClient.EXPECT().
					Cleanup(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Return(0, nil).Maybe()

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*spectrev1.ListenerList) = spectrev1.ListenerList{
							Items: []spectrev1.Listener{*listener},
						}
					}).
					Return(nil).Once()

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

				fakeClient.EXPECT().
					Cleanup(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Return(0, nil).Maybe()
				fakeClient.EXPECT().
					Cleanup(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Return(0, nil).Maybe()

				otherListener := newListener()
				otherListener.Name = "other-listener"
				otherListener.UID = "listener-uid-002"
				otherListener.Namespace = "other-team-ns"

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, opts ...client.ListOption) {
						for _, opt := range opts {
							Expect(opt).ToNot(BeAssignableToTypeOf(client.InNamespace("")),
								"generic Publisher ref-count must span all namespaces")
						}
						*list.(*spectrev1.ListenerList) = spectrev1.ListenerList{
							Items: []spectrev1.Listener{*listener, *otherListener},
						}
					}).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
