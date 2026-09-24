// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"
	"fmt"
	"reflect"

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
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/api/v1/builder"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
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
	testAppId          = "team-alpha--consumer-app"
	testRealmName      = "test-realm"
	testRealmIssuer    = "https://iris.example.com/auth/realms/test"
	testExposureUID    = "ae-uid-test-001"
	testExposureName   = "provider-app--api-v1-orders"
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
		Status: spectrev1.ListenerStatus{
			// Pre-set v2 so migration logic is skipped for non-migration tests.
			AuthorizationPolicyVersion: "v2",
		},
	}
}

// listenerControllerRef is the controller reference the approval controller
// writes on every scoped Approval it creates for the test Listener.
func listenerControllerRef() metav1.OwnerReference {
	return *metav1.NewControllerRef(newListener(), spectrev1.GroupVersion.WithKind("Listener"))
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

	// mockGetObserverApp stubs the observer Application Get. In the default
	// A==C case, the observer is the same as the consumer; call this after
	// mockGetConsumerApp to set up the second Get for the same name.
	mockGetObserverApp := func(app *applicationv1.Application) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: app.Name, Namespace: app.Namespace}, mock.AnythingOfType("*v1.Application")).
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

	// mockApprovalGrantedGateOwnedBy sets up the approval builder mock chain for a
	// single auto-granted approval gate whose Approval carries ownerRefs. The key
	// ("provider" or "consumer") is set on the returned Approval so the builder's
	// scoped identity check passes. The ApprovedRequest ref is captured from the
	// CreateOrUpdate call so isScopedGrantBound succeeds.
	mockApprovalGrantedGateOwnedBy := func(approvalKey string, ownerRefs []metav1.OwnerReference) {
		// Capture the ApprovalRequest so the Approval can reference it.
		var capturedAR approvalv1.ApprovalRequest

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				req := obj.(*approvalv1.ApprovalRequest)
				_ = mutate()
				if req.Spec.Strategy == approvalv1.ApprovalStrategyAuto {
					req.Spec.State = approvalv1.ApprovalStateGranted
				}
				capturedAR = *req.DeepCopy()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(nil).Once()

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Name = key.Name
				approval.Namespace = key.Namespace
				approval.OwnerReferences = ownerRefs
				approval.Spec.State = approvalv1.ApprovalStateGranted
				approval.Spec.ApprovalKey = approvalKey
				approval.Spec.Target = capturedAR.Spec.Target
				approval.Spec.ApprovedRequest = &ctypes.ObjectRef{
					Name:      capturedAR.Name,
					Namespace: capturedAR.Namespace,
					UID:       capturedAR.UID,
				}
			}).
			Return(nil).Once()
	}

	// mockApprovalGrantedGate is mockApprovalGrantedGateOwnedBy with the
	// controller reference the approval controller writes.
	mockApprovalGrantedGate := func(approvalKey string) {
		mockApprovalGrantedGateOwnedBy(approvalKey, []metav1.OwnerReference{listenerControllerRef()})
	}

	// mockApprovalGranted sets up both provider and consumer gates as auto-granted.
	mockApprovalGranted := func() {
		mockApprovalGrantedGate("provider") // provider gate
		mockApprovalGrantedGate("consumer") // consumer gate
	}

	// mockApprovalPendingGate sets up mock chain for a single pending gate.
	mockApprovalPendingGate := func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(nil).Once()

		// Get Approval — return NotFound (pending)
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Return(errors.NewNotFound(schema.GroupResource{Group: "approval.cp.ei.telekom.de", Resource: "approvals"}, "")).Once()
	}

	// mockApprovalPending sets up both gates as pending.
	mockApprovalPending := func() {
		mockApprovalPendingGate() // provider gate
		mockApprovalPendingGate() // consumer gate
	}

	// mockApprovalDeniedGate sets up mock chain for a single denied gate.
	mockApprovalDeniedGate := func(approvalKey string) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(nil).Once()

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Name = key.Name
				approval.Namespace = key.Namespace
				approval.OwnerReferences = []metav1.OwnerReference{listenerControllerRef()}
				approval.Spec.State = approvalv1.ApprovalStateRejected
				approval.Spec.ApprovalKey = approvalKey
				approval.Spec.Target = ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{Kind: "Listener", APIVersion: spectrev1.GroupVersion.String()},
					ObjectRef: ctypes.ObjectRef{Name: listenerName, Namespace: listenerNamespace, UID: "listener-uid-001"},
				}
			}).
			Return(nil).Once()
	}

	// mockApprovalDenied sets up provider gate as denied; consumer gate also denied
	// (aggregate is Denied if either gate is Denied).
	mockApprovalDenied := func() {
		mockApprovalDeniedGate("provider") // provider gate
		mockApprovalDeniedGate("consumer") // consumer gate
	}

	// mockApprovalRequestDeniedGate sets up mock chain for a single RequestDenied gate.
	mockApprovalRequestDeniedGate := func(approvalKey string) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				req := obj.(*approvalv1.ApprovalRequest)
				_ = mutate()
				req.Spec.State = approvalv1.ApprovalStateRejected
			}).
			Return(controllerutil.OperationResultNone, nil).Once()

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(nil).Once()

		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Name = key.Name
				approval.Namespace = key.Namespace
				approval.OwnerReferences = []metav1.OwnerReference{listenerControllerRef()}
				approval.Spec.State = approvalv1.ApprovalStateGranted
				approval.Spec.ApprovalKey = approvalKey
				approval.Spec.Target = ctypes.TypedObjectRef{
					TypeMeta:  metav1.TypeMeta{Kind: "Listener", APIVersion: spectrev1.GroupVersion.String()},
					ObjectRef: ctypes.ObjectRef{Name: listenerName, Namespace: listenerNamespace, UID: "listener-uid-001"},
				}
			}).
			Return(nil).Once()
	}

	// mockApprovalRequestDenied sets up both gates as request-denied.
	mockApprovalRequestDenied := func() {
		mockApprovalRequestDeniedGate("provider") // provider gate
		mockApprovalRequestDeniedGate("consumer") // consumer gate
	}

	// mockListRoutes stubs the gateway Route lookup and the subsequent
	// ApiExposure list call used by verifyProviderBinding.
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
						Labels: map[string]string{
							cconfig.OwnerUidLabelKey: testExposureUID,
						},
					},
					Spec: gatewayv1.RouteSpec{
						Paths: []string{"/gateway" + testApiBasePath},
					},
				}
			}).
			Return(nil).Once()

		// ApiExposure list for verifyProviderBinding — exposures live in the
		// provider's team namespace (listenerNamespace), not the zone namespace.
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiExposureList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*apiv1.ApiExposureList) = apiv1.ApiExposureList{
					Items: []apiv1.ApiExposure{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      testExposureName,
								Namespace: listenerNamespace,
								UID:       k8stypes.UID(testExposureUID),
								Labels: map[string]string{
									cconfig.BuildLabelKey("application"): providerAppName,
								},
							},
							Status: apiv1.ApiExposureStatus{
								Active: true,
								Route:  &ctypes.ObjectRef{Name: routeName, Namespace: listenerZoneStatus},
							},
						},
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

	// mockExplicitReadinessChecks stubs the Get calls that ensureChildReady
	// makes after AllReady() returns true. RouteListener and bridge Subscribers
	// are returned with Ready=True.
	mockListenerReadinessChecks := func() {
		// RouteListener readiness check.
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.RouteListener")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				rl := out.(*gatewayv1.RouteListener)
				meta.SetStatusCondition(&rl.Status.Conditions, metav1.Condition{
					Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
				})
			}).
			Return(nil).Once()
		// Bridge Subscriber readiness checks (2 subscribers: request + response).
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Subscriber")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				sub := out.(*pubsubv1.Subscriber)
				meta.SetStatusCondition(&sub.Status.Conditions, metav1.Condition{
					Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, Reason: "Ready",
				})
			}).
			Return(nil).Times(2)
	}

	setupFullHappyPath := func() *spectrev1.Listener {
		listener := newListener()
		mockGetConsumerApp(makeConsumerApp())
		mockGetProviderApp(makeProviderApp())
		mockGetSpectreApp(makeSpectreAppPtr())
		mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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

	// --- Multi-reconcile helpers ---
	//
	// These drive CreateOrUpdate several times in a row and carry the returned
	// Listener forward as the status the controller would have persisted.

	// reconcile runs CreateOrUpdate on a copy of l; the copy stands in for the
	// persisted object handed to the next reconcile.
	reconcile := func(l *spectrev1.Listener) (*spectrev1.Listener, error) {
		next := l.DeepCopy()
		err := h.CreateOrUpdate(ctx, next)
		return next, err
	}

	// countCalls counts client calls recorded since index from with the given
	// method whose object argument has the type of sample (nil matches any type).
	// Explicit counts are needed because some mocks (Subscriber CreateOrUpdate,
	// Zone, Realm, EventConfig list) are registered without Once().
	countCalls := func(from int, method string, sample client.Object) int {
		n := 0
		calls := fakeClient.Calls[from:]
		for i := range calls {
			if calls[i].Method != method {
				continue
			}
			if sample == nil || reflect.TypeOf(calls[i].Arguments.Get(1)) == reflect.TypeOf(sample) {
				n++
			}
		}
		return n
	}

	// expectNoCaptureCreates asserts that no RouteListener, Subscriber or
	// Publisher was created or updated since index from.
	expectNoCaptureCreates := func(from int) {
		Expect(countCalls(from, "CreateOrUpdate", &gatewayv1.RouteListener{})).To(BeZero())
		Expect(countCalls(from, "CreateOrUpdate", &pubsubv1.Subscriber{})).To(BeZero())
		Expect(countCalls(from, "CreateOrUpdate", &pubsubv1.Publisher{})).To(BeZero())
	}

	// expectRequestDenied asserts Ready=False/AccessDenied naming the gate whose
	// current ApprovalRequest was rejected.
	expectRequestDenied := func(l *spectrev1.Listener, gate string) {
		ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
		Expect(ready).ToNot(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal(condition.ReasonAccessDenied))
		Expect(ready.Message).To(ContainSubstring("ApprovalRequest"))
		Expect(ready.Message).To(ContainSubstring("(" + gate + " gate)"))
	}

	// mockEarlyRestrictionGranted stubs the step-0.5 reads of the Approvals
	// referenced by l's status, both Granted. Register it before the gate mocks:
	// testify matches expectations in registration order.
	mockEarlyRestrictionGranted := func(l *spectrev1.Listener) {
		for _, ref := range []*ctypes.ObjectRef{l.Status.ProviderApproval, l.Status.ConsumerApproval} {
			Expect(ref).ToNot(BeNil())
			fakeClient.EXPECT().
				Get(ctx, ref.K8s(), mock.AnythingOfType("*v1.Approval")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					out.(*approvalv1.Approval).Spec.State = approvalv1.ApprovalStateGranted
				}).
				Return(nil).Once()
		}
	}

	// mockResolveTopology stubs the Once() topology reads of one reconcile. Zone,
	// EventConfig and Realm reads stay registered from the first reconcile.
	mockResolveTopology := func() {
		mockGetConsumerApp(makeConsumerApp())
		mockGetProviderApp(makeProviderApp())
		mockGetSpectreApp(makeSpectreAppPtr())
		mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
		mockGetEventStore(makeListenerEventStore())
		mockListRoutes()
	}

	// mockOwnedLists stubs one owner-label List of RouteListeners and one of
	// Subscribers.
	mockOwnedLists := func(rls []gatewayv1.RouteListener, subs []pubsubv1.Subscriber) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				src := gatewayv1.RouteListenerList{Items: rls}
				src.DeepCopyInto(list.(*gatewayv1.RouteListenerList))
			}).
			Return(nil).Once()
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				src := pubsubv1.SubscriberList{Items: subs}
				src.DeepCopyInto(list.(*pubsubv1.SubscriberList))
			}).
			Return(nil).Once()
	}

	// mockRejectedGates stubs the dual-gate build with the given gate's current
	// ApprovalRequest Rejected and the other gate Granted.
	mockRejectedGates := func(gate string) {
		for _, key := range []string{"provider", "consumer"} {
			if key == gate {
				mockApprovalRequestDeniedGate(key)
			} else {
				mockApprovalGrantedGate(key)
			}
		}
	}

	// mockGateBoundToOldRequest stubs one gate whose Granted Approval is still
	// bound to oldReq, so the builder reports Pending for a new-intent request.
	// The created ApprovalRequest is captured after mutate.
	mockGateBoundToOldRequest := func(approvalKey string, oldReq *ctypes.ObjectRef, captured *approvalv1.ApprovalRequest) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				req := obj.(*approvalv1.ApprovalRequest)
				_ = mutate()
				req.DeepCopyInto(captured)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
			Return(nil).Once()

		isController := true
		fakeClient.EXPECT().
			Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
			Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				approval := out.(*approvalv1.Approval)
				approval.Name = key.Name
				approval.Namespace = key.Namespace
				approval.OwnerReferences = []metav1.OwnerReference{{
					APIVersion: spectrev1.GroupVersion.String(),
					Kind:       "Listener",
					Name:       listenerName,
					UID:        "listener-uid-001",
					Controller: &isController,
				}}
				approval.Spec.State = approvalv1.ApprovalStateGranted
				approval.Spec.ApprovalKey = approvalKey
				approval.Spec.Target = captured.Spec.Target
				approval.Spec.ApprovedRequest = oldReq.DeepCopy()
			}).
			Return(nil).Once()
	}

	// rejectionFixture is a provisioned Listener plus the live children an
	// owner-label List would return for it.
	type rejectionFixture struct {
		listener    *spectrev1.Listener
		fingerprint string
		rl          gatewayv1.RouteListener
		subs        []pubsubv1.Subscriber
		providerReq *ctypes.ObjectRef
		consumerReq *ctypes.ObjectRef
	}

	// provisionForRejection runs R0 with both gates granted and builds the live
	// children from the persisted status refs. They carry the applied
	// fingerprint label so removeStaleChildren keeps them.
	provisionForRejection := func() *rejectionFixture {
		fakeClient.EXPECT().AnyChanged().Return(true).Once()
		l, err := reconcile(setupFullHappyPath())
		Expect(err).ToNot(HaveOccurred())
		Expect(l.Status.AppliedPlacement).ToNot(BeNil())
		fp := l.Status.AppliedPlacement.Fingerprint
		Expect(fp).ToNot(BeEmpty())
		Expect(l.Status.RouteListener).ToNot(BeNil())
		Expect(l.Status.EventSubscriptions).To(HaveLen(2))
		Expect(l.Status.ProviderApproval).ToNot(BeNil())
		Expect(l.Status.ConsumerApproval).ToNot(BeNil())
		Expect(l.Status.ProviderApprovalRequest).ToNot(BeNil())
		Expect(l.Status.ConsumerApprovalRequest).ToNot(BeNil())

		labels := func() map[string]string {
			return map[string]string{
				cconfig.OwnerUidLabelKey:                 "listener-uid-001",
				handler.AuthorizationFingerprintLabelKey: fp,
			}
		}
		f := &rejectionFixture{
			listener:    l,
			fingerprint: fp,
			rl: gatewayv1.RouteListener{ObjectMeta: metav1.ObjectMeta{
				Name:            l.Status.RouteListener.Name,
				Namespace:       l.Status.RouteListener.Namespace,
				UID:             "rl-uid-1",
				ResourceVersion: "7",
				Labels:          labels(),
			}},
			providerReq: l.Status.ProviderApprovalRequest.DeepCopy(),
			consumerReq: l.Status.ConsumerApprovalRequest.DeepCopy(),
		}
		for i, ref := range l.Status.EventSubscriptions {
			f.subs = append(f.subs, pubsubv1.Subscriber{ObjectMeta: metav1.ObjectMeta{
				Name:            ref.Name,
				Namespace:       ref.Namespace,
				UID:             k8stypes.UID(fmt.Sprintf("sub-uid-%d", i)),
				ResourceVersion: "3",
				Labels:          labels(),
			}})
		}
		return f
	}

	// provisionAndDrainAfterRejection provisions a Listener (R0), rejects the
	// given gate's current ApprovalRequest and drives the persisted drain to
	// completion (R1-R6), asserting the checkpoint and delete order on the way.
	provisionAndDrainAfterRejection := func(gate string) *rejectionFixture {
		f := provisionForRejection()
		otherGate := "consumer"
		if gate == "consumer" {
			otherGate = "provider"
		}
		liveRLs := []gatewayv1.RouteListener{f.rl}
		rlKey := k8stypes.NamespacedName{Name: f.rl.Name, Namespace: f.rl.Namespace}

		// R1: the gate's current request is Rejected. The drain checkpoint is
		// written and the reconcile returns before anything is deleted.
		preR1 := f.listener
		mockR1 := func() {
			mockEarlyRestrictionGranted(preR1)
			mockResolveTopology()
			mockOwnedLists(liveRLs, f.subs) // removeStaleChildren: same fingerprint, kept
			mockRejectedGates(gate)
			mockOwnedLists(liveRLs, f.subs) // startDrain snapshot
		}
		expectCheckpoint := func(l *spectrev1.Listener, from int) {
			d := l.Status.Draining
			Expect(d).ToNot(BeNil())
			Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
			Expect(d.Reason).To(Equal("approval request rejected (" + gate + " gate)"))
			Expect(d.OldFingerprint).To(Equal(f.fingerprint))
			Expect(d.OldRouteListener).ToNot(BeNil())
			Expect(d.OldRouteListener.Name).To(Equal(f.rl.Name))
			Expect(d.OldRouteListener.UID).To(Equal(k8stypes.UID("rl-uid-1")))
			Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&f.rl)}))
			// Sorted by namespace/name: the "--rp" Subscriber (sub-uid-1) sorts
			// before the "--rq" one (sub-uid-0).
			Expect(d.OldSubscribers).To(Equal([]ctypes.ObjectRef{
				*ctypes.ObjectRefFromObject(&f.subs[1]),
				*ctypes.ObjectRefFromObject(&f.subs[0]),
			}))
			Expect(d.OldSubscribers[0].UID).To(Equal(k8stypes.UID("sub-uid-1")))
			Expect(d.OldSubscribers[1].UID).To(Equal(k8stypes.UID("sub-uid-0")))
			Expect(d.SourcePublisher).ToNot(BeNil())
			Expect(d.SourcePublisher).To(Equal(l.Status.AppliedPlacement.Publisher))

			// Capture is still recorded: nothing was deleted yet.
			Expect(l.Status.RouteListener).ToNot(BeNil())
			Expect(l.Status.EventSubscriptions).To(HaveLen(2))
			Expect(l.Status.AppliedPlacement.Fingerprint).To(Equal(f.fingerprint))
			Expect(countCalls(from, "Delete", nil)).To(BeZero())
			expectNoCaptureCreates(from)

			expectRequestDenied(l, gate)
			rejected := meta.FindStatusCondition(l.Status.Conditions, builder.ConditionTypeForKey(gate))
			Expect(rejected).ToNot(BeNil())
			Expect(rejected.Status).To(Equal(metav1.ConditionFalse))
			Expect(rejected.Reason).To(Equal(string(approvalv1.ApprovalStateRejected)))
			granted := meta.FindStatusCondition(l.Status.Conditions, builder.ConditionTypeForKey(otherGate))
			Expect(granted).ToNot(BeNil())
			Expect(granted.Status).To(Equal(metav1.ConditionTrue))
		}

		mockR1()
		start := len(fakeClient.Calls)
		l, err := reconcile(preR1)
		Expect(err).ToNot(HaveOccurred())
		expectCheckpoint(l, start)

		// R1b: the R1 status write was lost. Re-running from the pre-R1 object
		// must again only write a checkpoint.
		mockR1()
		start = len(fakeClient.Calls)
		l, err = reconcile(preR1)
		Expect(err).ToNot(HaveOccurred())
		expectCheckpoint(l, start)

		// R2: Stopping deletes the old RouteListener with UID+RV preconditions.
		fakeClient.EXPECT().
			Get(ctx, rlKey, mock.AnythingOfType("*v1.RouteListener")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				f.rl.DeepCopyInto(out.(*gatewayv1.RouteListener))
			}).
			Return(nil).Once()
		fakeClient.EXPECT().
			Delete(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, opts ...client.DeleteOption) {
				do := (&client.DeleteOptions{}).ApplyOptions(opts)
				Expect(do.Preconditions).ToNot(BeNil())
				Expect(do.Preconditions.UID).To(HaveValue(Equal(k8stypes.UID("rl-uid-1"))))
				Expect(do.Preconditions.ResourceVersion).To(HaveValue(Equal("7")))
			}).
			Return(nil).Once()
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(Equal(1))
		Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
		Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(BeZero())
		expectNoCaptureCreates(start)
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))

		// R3: the RouteListener is gone; its ref is cleared before any Subscriber
		// is touched.
		fakeClient.EXPECT().
			Get(ctx, rlKey, mock.AnythingOfType("*v1.RouteListener")).
			Return(errors.NewNotFound(schema.GroupResource{Group: gatewayv1.GroupVersion.Group, Resource: "routelisteners"}, f.rl.Name)).Once()
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(countCalls(start, "Delete", nil)).To(BeZero())
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
		Expect(l.Status.RouteListener).To(BeNil())
		Expect(l.Status.EventSubscriptions).To(HaveLen(2))

		// R4: DrainingSubscribers deletes both Subscribers with UID+RV preconditions.
		subUIDs := []k8stypes.UID{f.subs[0].UID, f.subs[1].UID}
		for i := range f.subs {
			sub := f.subs[i].DeepCopy()
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: sub.Name, Namespace: sub.Namespace}, mock.AnythingOfType("*v1.Subscriber")).
				Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
					sub.DeepCopyInto(out.(*pubsubv1.Subscriber))
				}).
				Return(nil).Once()
		}
		fakeClient.EXPECT().
			Delete(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, opts ...client.DeleteOption) {
				do := (&client.DeleteOptions{}).ApplyOptions(opts)
				Expect(subUIDs).To(ContainElement(obj.GetUID()))
				Expect(do.Preconditions).ToNot(BeNil())
				Expect(do.Preconditions.UID).To(HaveValue(Equal(obj.GetUID())))
				Expect(do.Preconditions.ResourceVersion).To(HaveValue(Equal("3")))
			}).
			Return(nil).Times(2)
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(Equal(2))
		Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
		Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(BeZero())
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))

		// R5: both Subscribers are gone.
		for i := range f.subs {
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: f.subs[i].Name, Namespace: f.subs[i].Namespace}, mock.AnythingOfType("*v1.Subscriber")).
				Return(errors.NewNotFound(schema.GroupResource{Group: pubsubv1.GroupVersion.Group, Resource: "subscribers"}, f.subs[i].Name)).Once()
		}
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(countCalls(start, "Delete", nil)).To(BeZero())
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
		Expect(l.Status.EventSubscriptions).To(BeEmpty())

		// R6: CleaningPublisher removes the orphaned generic Publisher, the drain
		// completes and the same reconcile falls through: the request is still
		// Rejected and nothing is applied, so the applied placement is cleared.
		src := l.Status.Draining.SourcePublisher.DeepCopy()
		Expect(src).ToNot(BeNil())
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
			Return(nil).Once()
		fakeClient.EXPECT().
			Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, _ ...client.DeleteOption) {
				Expect(obj.GetName()).To(Equal(src.Name))
				Expect(obj.GetNamespace()).To(Equal(src.Namespace))
			}).
			Return(nil).Once()
		mockEarlyRestrictionGranted(l)
		mockResolveTopology()
		mockOwnedLists(nil, nil) // removeStaleChildren
		mockRejectedGates(gate)
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(l.Status.Draining).To(BeNil())
		Expect(l.Status.AppliedPlacement).To(BeNil())
		Expect(l.Status.RouteListener).To(BeNil())
		Expect(l.Status.EventSubscriptions).To(BeEmpty())
		Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1))
		Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
		Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
		expectNoCaptureCreates(start)
		expectRequestDenied(l, gate)

		f.listener = l
		return f
	}

	// expectStaysDrainedWhileRejected runs several more reconciles with the same
	// rejection and asserts that no drain restarts and no capture is recreated.
	// Each one takes stopCapture's direct branch: no owned children are found and
	// the Publisher namespace falls back to the consumer zone.
	expectStaysDrainedWhileRejected := func(l *spectrev1.Listener, gate string) {
		for range 3 {
			mockEarlyRestrictionGranted(l)
			mockResolveTopology()
			mockOwnedLists(nil, nil) // removeStaleChildren
			mockRejectedGates(gate)
			mockOwnedLists(nil, nil)              // deleteAllOwnedChildren
			mockOwnedLists(nil, nil)              // resolvePublisherNamespace
			mockGetConsumerApp(makeConsumerApp()) // namespace fallback via consumer zone
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Return(nil).Once()
			fakeClient.EXPECT().
				Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
				Return(errors.NewNotFound(schema.GroupResource{Group: pubsubv1.GroupVersion.Group, Resource: "publishers"}, "")).Once()

			start := len(fakeClient.Calls)
			next, err := reconcile(l)
			Expect(err).ToNot(HaveOccurred())
			Expect(next.Status.Draining).To(BeNil())
			Expect(next.Status.AppliedPlacement).To(BeNil())
			Expect(next.Status.RouteListener).To(BeNil())
			Expect(next.Status.EventSubscriptions).To(BeEmpty())
			Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
			Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
			expectNoCaptureCreates(start)
			expectRequestDenied(next, gate)
			l = next
		}
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

		Context("when observer (A) differs from consumer (C)", func() {
			It("should resolve observer independently and proceed to zone resolution", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())

				// SpectreApplication references a different Application than the Listener's consumer.
				observerAppName := "observer-app"
				sa := makeSpectreAppPtr()
				sa.Spec.Application.ObjectRef = ctypes.ObjectRef{
					Name:      observerAppName,
					Namespace: listenerNamespace,
				}
				sa.Status.Id = "team-gamma--observer-app"
				mockGetSpectreApp(sa)

				// The handler will resolve the observer Application.
				observerApp := &applicationv1.Application{
					ObjectMeta: metav1.ObjectMeta{
						Name:      observerAppName,
						Namespace: listenerNamespace,
						UID:       "observer-uid-001",
					},
					Spec: applicationv1.ApplicationSpec{
						Team:      "team-gamma",
						TeamEmail: "gamma@test.com",
						Zone:      ctypes.ObjectRef{Name: listenerZoneName, Namespace: listenerZoneNs},
					},
					Status: applicationv1.ApplicationStatus{
						ClientId: "team-gamma--observer-app",
					},
				}
				meta.SetStatusCondition(&observerApp.Status.Conditions, metav1.Condition{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Ready",
				})
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: observerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						*out.(*applicationv1.Application) = *observerApp
					}).
					Return(nil).Once()

				// Observer zone resolution.
				mockGetZone()
				// Consumer + provider zone resolution.
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
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockListenerReadinessChecks()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCond.Reason).To(Equal(condition.ReasonProvisioned))
			})
		})

		Context("when approvals are pending", func() {
			It("should set Blocked condition and NOT create downstream resources", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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

				// After deleteAllOwnedChildren, resolvePublisherNamespace is called.
				// Status refs are nil — fall back to owner-labelled children. Return
				// one RouteListener so the namespace is resolved without topology.
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
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// cleanupGenericPublisherIfOrphaned: no Subscribers reference the Publisher.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
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
			It("should set AccessDenied naming both gates and delete no capture children when nothing is applied", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalRequestDenied()

				// stopCapture direct branch (nothing applied): deleteAllOwnedChildren
				// and resolvePublisherNamespace find no owned children, so the
				// namespace falls back to the consumer zone for the Publisher check.
				mockOwnedLists(nil, nil)
				mockOwnedLists(nil, nil)
				mockGetConsumerApp(makeConsumerApp())
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// AccessDenied condition must be set.
				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(readyCond.Message).To(ContainSubstring("provider and consumer"))

				Expect(listener.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
				Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
				expectNoCaptureCreates(start)
			})
		})

		Context("when approvals are granted", func() {
			It("should create RouteListener with correct fields and fingerprint label", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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
				mockListenerReadinessChecks()

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
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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
				mockListenerReadinessChecks()

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
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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
				mockListenerReadinessChecks()

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
				mockListenerReadinessChecks()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCond.Reason).To(Equal(condition.ReasonProvisioned))

				// Verify AppliedPlacement was written on successful provisioning.
				Expect(listener.Status.AppliedPlacement).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.Fingerprint).ToNot(BeEmpty())
				Expect(listener.Status.AppliedPlacement.CaptureRoute).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.CaptureZone).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.CaptureEventStore).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.DeliveryZone).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.DeliveryEventStore).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.CallbackOriginZone).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.Publisher).ToNot(BeNil())
				Expect(listener.Status.AppliedPlacement.CallbackBaseURL).To(Equal(testCallbackURL))
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
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()

				// Capture the first ApprovalRequest (provider gate) to inspect properties.
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
					Return(controllerutil.OperationResultCreated, nil).Once()

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
					Return(nil).Once()

				fakeClient.EXPECT().
					Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, key k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Name = key.Name
						approval.Namespace = key.Namespace
						approval.OwnerReferences = []metav1.OwnerReference{listenerControllerRef()}
						approval.Spec.State = approvalv1.ApprovalStateGranted
						approval.Spec.ApprovalKey = "provider"
						approval.Spec.Target = capturedReq.Spec.Target
						approval.Spec.ApprovedRequest = &ctypes.ObjectRef{
							Name:      capturedReq.Name,
							Namespace: capturedReq.Namespace,
							UID:       capturedReq.UID,
						}
					}).
					Return(nil).Once()

				// Consumer gate (second) — auto-granted like provider.
				mockApprovalGrantedGate("consumer")

				mockCreateOrUpdatePublisher()
				mockGetRealm()
				mockCreateOrUpdateRouteListener()
				mockCreateOrUpdateSubscriber()
				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockListenerReadinessChecks()

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
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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

		Context("when listener has filters", func() {
			It("should map requestFilter trigger to SelectionFilter and responseFilter payload to ResponseFilter", func() {
				listener := newListener()
				listener.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{
					Trigger: map[string]string{"method": "POST"},
				}
				listener.Spec.ApiListener.ResponseFilter = &spectrev1.ListenerFilter{
					Payload: []string{"body.name", "body.status"},
				}
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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
				mockListenerReadinessChecks()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(capturedSubs).To(HaveLen(2))

				// Request subscriber: user trigger entries merged with fixed attrs.
				rqSub := capturedSubs[0]
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("REQUEST"))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["issue"]).To(Equal(testApiBasePath))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["consumer"]).To(Equal(consumerClientId))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["provider"]).To(Equal(providerClientId))
				Expect(rqSub.Spec.Trigger.SelectionFilter.Attributes["method"]).To(Equal("POST"))
				Expect(rqSub.Spec.Trigger.ResponseFilter).To(BeNil())

				// Response subscriber: payload entries mapped with payload. prefix.
				rpSub := capturedSubs[1]
				Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["kind"]).To(Equal("RESPONSE"))
				Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["issue"]).To(Equal(testApiBasePath))
				Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["consumer"]).To(Equal(consumerClientId))
				Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes["provider"]).To(Equal(providerClientId))
				Expect(rpSub.Spec.Trigger.SelectionFilter.Attributes).ToNot(HaveKey("method"))
				Expect(rpSub.Spec.Trigger.ResponseFilter).ToNot(BeNil())
				Expect(rpSub.Spec.Trigger.ResponseFilter.Paths).To(Equal([]string{"payload.body.name", "payload.body.status"}))
				Expect(rpSub.Spec.Trigger.ResponseFilter.Mode).To(Equal(pubsubv1.ResponseFilterModeInclude))
			})
		})

		Context("unsupported route modes", func() {
			// In these tests, the route mode is rejected (pass-through/failover) before
			// placement resolution. The cleanup deletes children and checks the generic
			// Publisher. resolvePublisherNamespace falls back to the consumer zone via
			// the topology resolver.
			It("should block with pass-through route and NOT create ApprovalRequest or children", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				// No mockGetObserverApp: route mode check blocks before observer resolution.
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
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

				// resolvePublisherNamespace: topology fallback.
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
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// cleanupGenericPublisherIfOrphaned: no Subscribers reference the Publisher.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
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
				// No mockGetObserverApp: route mode check blocks before observer resolution.
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
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

				// resolvePublisherNamespace: topology fallback.
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
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// cleanupGenericPublisherIfOrphaned: no Subscribers reference the Publisher.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
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
				// No mockGetEventStore: ResolvePlacement is never reached because
				// the pass-through route check rejects before placement resolution.
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

				// resolvePublisherNamespace: status refs nil — label-list returns one
				// RouteListener so namespace resolves without topology.
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
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// cleanupGenericPublisherIfOrphaned: no Subscribers reference the Publisher.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
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

		// --- Dual-gate aggregate pair table (Task 5, brief section 1) ---
		// These handler-level tests verify the handler's switch statement and
		// side effects (cleanup, conditions, provisioning) for each aggregate
		// outcome. The pure-function tests live in listener_approval_test.go.

		Context("dual-gate: provider Granted + consumer Pending", func() {
			It("should NOT provision and set Blocked condition", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalGrantedGate("provider")
				mockApprovalPendingGate()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				procCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(procCond).ToNot(BeNil())
				Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))

				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})
		})

		Context("dual-gate: provider Denied + consumer Granted", func() {
			It("should cleanup all children and set AccessDenied", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				}

				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalDeniedGate("provider")
				mockApprovalGrantedGate("consumer")

				// Denial cleanup: deleteAllOwnedChildren.
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

				// resolvePublisherNamespace: label-list returns RL.
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
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
			})
		})

		Context("dual-gate: provider Granted + consumer Denied", func() {
			It("should cleanup all children and set AccessDenied", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalGrantedGate("provider")
				mockApprovalDeniedGate("consumer")

				// Denial cleanup: deleteAllOwnedChildren (no children).
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

				// resolvePublisherNamespace: topology fallback.
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
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
			})
		})

		Context("dual-gate: provider Error + consumer Pending", func() {
			It("should return error (NOT Pending) and NOT provision", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()

				// Provider gate: Approval Get returns an internal error (not NotFound).
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
					Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
					Return(nil).Once()
				fakeClient.EXPECT().
					Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
					Return(fmt.Errorf("internal API error")).Once()

				// Consumer gate: pending.
				mockApprovalPendingGate()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("approval evaluation failed"))

				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})
		})

		// Section 2.6: a persisted scoped Approval authorizes capture only when this
		// Listener is its controller owner, as the approval controller writes it.
		Context("scoped identity: persisted Approval without a controller owner", func() {
			It("does not provision capture across consecutive reconciles", func() {
				l := newListener()
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})

				for i := range 2 {
					if i > 0 {
						// Step 0.5 reads the Approvals the previous reconcile recorded.
						mockEarlyRestrictionGranted(l)
					}
					mockResolveTopology()
					mockNoStaleChildren()
					mockApprovalGrantedGateOwnedBy("provider", nil)
					mockApprovalGrantedGate("consumer")

					var err error
					l, err = reconcile(l)
					Expect(err).To(MatchError(ContainSubstring("missing or foreign controller owner")))
					Expect(err.Error()).To(ContainSubstring("approval evaluation failed"))
					Expect(l.Status.RouteListener).To(BeNil())
					Expect(l.Status.EventSubscriptions).To(BeEmpty())
					Expect(l.Status.AppliedPlacement).To(BeNil())
					Expect(l.Status.ProviderApproval).ToNot(BeNil())
				}

				expectNoCaptureCreates(0)
				Expect(countCalls(0, "Delete", nil)).To(BeZero())
			})
		})

		Context("dual-gate: both Pending", func() {
			It("should NOT provision, stale children already removed", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
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

		// Spectre policy (plan 2.3): a rejected current ApprovalRequest stops and
		// drains this Listener's capture through the persisted drain.
		Context("dual-gate: provider RequestDenied + consumer Granted", func() {
			It("checkpoints before deleting, drains the RouteListener then Subscribers, and stays drained", func() {
				f := provisionAndDrainAfterRejection("provider")
				expectStaysDrainedWhileRejected(f.listener, "provider")
			})
		})

		Context("dual-gate: provider Granted + consumer RequestDenied", func() {
			It("checkpoints before deleting, drains the RouteListener then Subscribers, and stays drained", func() {
				f := provisionAndDrainAfterRejection("consumer")
				expectStaysDrainedWhileRejected(f.listener, "consumer")
			})
		})

		Context("dual-gate: provider RequestDenied + consumer Error", func() {
			It("persists a drain checkpoint and returns the consumer gate error", func() {
				f := provisionForRejection()
				liveRLs := []gatewayv1.RouteListener{f.rl}

				mockEarlyRestrictionGranted(f.listener)
				mockResolveTopology()
				mockOwnedLists(liveRLs, f.subs) // removeStaleChildren
				mockApprovalRequestDeniedGate("provider")
				// Consumer gate: Approval Get returns an internal error.
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
					Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
					Return(nil).Once()
				fakeClient.EXPECT().
					Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
					Return(fmt.Errorf("internal API error")).Once()
				mockOwnedLists(liveRLs, f.subs) // startDrain snapshot

				start := len(fakeClient.Calls)
				l, err := reconcile(f.listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer gate"))
				Expect(err.Error()).To(ContainSubstring("internal API error"))

				Expect(l.Status.Draining).ToNot(BeNil())
				Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(l.Status.Draining.OldFingerprint).To(Equal(f.fingerprint))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
				expectRequestDenied(l, "provider")
			})
		})

		Context("request denial: intent change after drain", func() {
			It("creates and evaluates the new-intent ApprovalRequests and provisions once both gates grant", func() {
				f := provisionAndDrainAfterRejection("provider")
				l := f.listener
				l.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{Trigger: map[string]string{"method": "POST"}}

				// R7: the changed intent yields new ApprovalRequest names. The
				// Approvals are still bound to the rejected V1 requests, so both
				// gates are Pending (not AccessDenied).
				var capP, capC approvalv1.ApprovalRequest
				mockEarlyRestrictionGranted(l)
				mockResolveTopology()
				mockOwnedLists(nil, nil) // removeStaleChildren
				mockGateBoundToOldRequest("provider", f.providerReq, &capP)
				mockGateBoundToOldRequest("consumer", f.consumerReq, &capC)

				start := len(fakeClient.Calls)
				l7, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())

				Expect(capP.Name).To(HavePrefix("ar-v1-"))
				Expect(capP.Name).ToNot(Equal(f.providerReq.Name))
				Expect(l7.Status.ProviderApprovalRequest).ToNot(BeNil())
				Expect(l7.Status.ProviderApprovalRequest.Name).To(Equal(capP.Name))
				Expect(capC.Name).To(HavePrefix("ar-v1-"))
				Expect(capC.Name).ToNot(Equal(f.consumerReq.Name))
				Expect(l7.Status.ConsumerApprovalRequest).ToNot(BeNil())
				Expect(l7.Status.ConsumerApprovalRequest.Name).To(Equal(capC.Name))

				ready := meta.FindStatusCondition(l7.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready).ToNot(BeNil())
				Expect(ready.Reason).To(Equal(condition.ReasonApprovalPending))
				Expect(l7.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)

				// R8: both gates grant the V2 requests; provisioning resumes under
				// the new fingerprint.
				mockEarlyRestrictionGranted(l7)
				mockResolveTopology()
				mockOwnedLists(nil, nil) // removeStaleChildren
				mockApprovalGranted()
				mockCreateOrUpdatePublisher()
				var capturedRL *gatewayv1.RouteListener
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
						capturedRL = obj.(*gatewayv1.RouteListener).DeepCopy()
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()
				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(true).Once()

				start = len(fakeClient.Calls)
				l8, err := reconcile(l7)
				Expect(err).ToNot(HaveOccurred())

				Expect(l8.Status.AppliedPlacement).ToNot(BeNil())
				Expect(l8.Status.AppliedPlacement.Fingerprint).ToNot(Equal(f.fingerprint))
				Expect(capturedRL).ToNot(BeNil())
				Expect(capturedRL.Labels).To(HaveKeyWithValue(handler.AuthorizationFingerprintLabelKey, l8.Status.AppliedPlacement.Fingerprint))
				Expect(l8.Status.RouteListener).ToNot(BeNil())
				Expect(l8.Status.EventSubscriptions).To(HaveLen(2))
				Expect(countCalls(start, "CreateOrUpdate", &gatewayv1.RouteListener{})).To(Equal(1))
				Expect(countCalls(start, "CreateOrUpdate", &pubsubv1.Subscriber{})).To(Equal(2))
				Expect(countCalls(start, "CreateOrUpdate", &pubsubv1.Publisher{})).To(Equal(1))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())

				ready = meta.FindStatusCondition(l8.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready).ToNot(BeNil())
				Expect(ready.Reason).To(Equal(condition.ReasonSubResourceNotReady))
			})
		})

		Context("dual-gate: both Granted provisions downstream resources", func() {
			It("should create RouteListener and Subscribers when both gates grant", func() {
				listener := setupFullHappyPath()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockListenerReadinessChecks()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(listener.Status.RouteListener).ToNot(BeNil())
				Expect(listener.Status.EventSubscriptions).ToNot(BeEmpty())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(readyCond.Reason).To(Equal(condition.ReasonProvisioned))
			})
		})

		// --- Brief section 4: Denial plus other-gate error ---

		Context("dual-gate: provider Denied + consumer Error", func() {
			It("should still trigger cleanup and return combined error", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}

				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()

				// Provider gate: denied.
				mockApprovalDeniedGate("provider")

				// Consumer gate: error (internal API error on Approval Get).
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
					Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
					}).
					Return(controllerutil.OperationResultCreated, nil).Once()
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
					Return(nil).Once()
				fakeClient.EXPECT().
					Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
					Return(fmt.Errorf("consumer API error")).Once()

				// Denial cleanup: deleteAllOwnedChildren.
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
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// resolvePublisherNamespace: label-list returns RL.
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
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				// Denial cleanup succeeds, but the combined error from the consumer
				// gate is preserved and returned.
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer gate"))

				// Cleanup still happened.
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())

				// Provider gate was denied (successful fetch) — refs populated.
				Expect(listener.Status.ProviderApproval).ToNot(BeNil())
				Expect(listener.Status.ProviderApprovalRequest).ToNot(BeNil())

				// Consumer gate errored during Approval Get, but the builder
				// computes the Approval name deterministically and the AR was
				// created before the error — both refs are still populated.
				Expect(listener.Status.ConsumerApproval).ToNot(BeNil())
				Expect(listener.Status.ConsumerApprovalRequest).ToNot(BeNil())
			})
		})

		// --- Brief section 5: No capture from partial authorization ---

		Context("dual-gate: no provisioning from partial authorization", func() {
			It("should NOT create RouteListener or Subscribers when only provider is Granted", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalGrantedGate("provider")
				mockApprovalPendingGate() // consumer pending

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// No downstream resources created.
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())

				// Blocked condition set.
				procCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(procCond).ToNot(BeNil())
				Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))
			})

			It("should NOT create RouteListener or Subscribers when only consumer is Granted", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalPendingGate() // provider pending
				mockApprovalGrantedGate("consumer")

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})

			It("should NOT create RouteListener or Subscribers when one Granted + one Denied", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				mockApprovalGrantedGate("provider")
				mockApprovalDeniedGate("consumer")

				// Denial cleanup (no children).
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

				// resolvePublisherNamespace: topology fallback.
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
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// No provisioning happened.
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
			})
		})

		// --- Brief section 3: Identity and team combinations (handler-level) ---
		// A!=C is now supported (Task 7). All team combinations are reachable.
		// The pure-function tests live in listener_approval_test.go.

		Context("dual-gate: all teams equal (A==C==P) auto-grants both", func() {
			It("should provision when observer/consumer/provider share a team", func() {
				listener := newListener()
				// Set all apps to the same team.
				consumerApp := makeConsumerApp()
				consumerApp.Spec.Team = "shared-team"
				providerApp := makeProviderApp()
				providerApp.Spec.Team = "shared-team"

				mockGetConsumerApp(consumerApp)
				mockGetProviderApp(providerApp)
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(consumerApp) // A==C: observer resolves to consumer (same team override)
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				// Both gates auto-granted because requester team == decider team.
				mockApprovalGranted()
				mockCreateOrUpdatePublisher()
				mockGetRealm()
				mockCreateOrUpdateRouteListener()
				mockCreateOrUpdateSubscriber()
				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockListenerReadinessChecks()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		// Golden values captured at 032f2a57 for a same-team, single-zone A==C
		// Listener whose capture, delivery and callback origin are one local zone.
		// Placement changes must not alter them, or every existing Listener would
		// be drained and re-approved on upgrade.
		Context("fingerprint stability vs 032f2a57", func() {
			const (
				goldenFingerprint     = "a89666ae23f97439136fb1806adb3c747e87d57353a2cec710874c49ff6929e"
				goldenProviderRequest = "ar-v1-c1d2aba1131cd6a6a86e2b8be16d4e1b0bd85ed1551f61be"
				goldenConsumerRequest = "ar-v1-eaad806d5f758ab59d275ff8981c4fcaf616924dd1f3efbb"
			)

			sameTeamApps := func() (*applicationv1.Application, *applicationv1.Application) {
				consumerApp := makeConsumerApp()
				consumerApp.Spec.Team = "shared-team"
				providerApp := makeProviderApp()
				providerApp.Spec.Team = "shared-team"
				return consumerApp, providerApp
			}

			// mockReadyReconcile stubs one reconcile that provisions (or re-applies)
			// capture and reports Ready. children are returned by removeStaleChildren.
			mockReadyReconcile := func(rls []gatewayv1.RouteListener, subs []pubsubv1.Subscriber) {
				consumerApp, providerApp := sameTeamApps()
				mockGetConsumerApp(consumerApp)
				mockGetProviderApp(providerApp)
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(consumerApp)
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockOwnedLists(rls, subs)
				mockApprovalGranted()
				mockCreateOrUpdatePublisher()
				mockGetRealm()
				mockCreateOrUpdateRouteListener()
				mockCreateOrUpdateSubscriber()
				mockJanitorCleanup()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockListenerReadinessChecks()
			}

			expectGolden := func(l *spectrev1.Listener) {
				Expect(l.Status.AppliedPlacement).ToNot(BeNil())
				Expect(l.Status.AppliedPlacement.Fingerprint).To(Equal(goldenFingerprint))
				Expect(l.Status.ProviderApprovalRequest).ToNot(BeNil())
				Expect(l.Status.ProviderApprovalRequest.Name).To(Equal(goldenProviderRequest))
				Expect(l.Status.ConsumerApprovalRequest).ToNot(BeNil())
				Expect(l.Status.ConsumerApprovalRequest.Name).To(Equal(goldenConsumerRequest))
				ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready).ToNot(BeNil())
				Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			}

			It("T1: same-team single-zone A==C keeps fingerprint and scoped request names", func() {
				mockReadyReconcile(nil, nil)
				start := len(fakeClient.Calls)

				l, err := reconcile(newListener())
				Expect(err).ToNot(HaveOccurred())
				expectGolden(l)
				var rlLabels []string
				for _, call := range fakeClient.Calls[start:] {
					if call.Method != "CreateOrUpdate" {
						continue
					}
					if rl, ok := call.Arguments.Get(1).(*gatewayv1.RouteListener); ok {
						rlLabels = append(rlLabels, rl.Labels[handler.AuthorizationFingerprintLabelKey])
					}
				}
				Expect(rlLabels).To(Equal([]string{goldenFingerprint}))
				Expect(l.Status.AppliedPlacement.CaptureZone.Name).To(Equal(listenerZoneName))
				Expect(l.Status.AppliedPlacement.DeliveryZone.Name).To(Equal(listenerZoneName))
				Expect(l.Status.AppliedPlacement.CallbackOriginZone.Name).To(Equal(listenerZoneName))
				Expect(l.Status.AppliedPlacement.CallbackBaseURL).To(Equal(testCallbackURL))
			})

			It("T2: status persisted by 032f2a57 converges without drain or deletion", func() {
				l := newListener()
				zoneRef := &ctypes.ObjectRef{Name: listenerZoneName, Namespace: listenerZoneNs}
				esRef := &ctypes.ObjectRef{Name: "eventstore-aws", Namespace: listenerZoneStatus}
				rlName := util.MakeRouteListenerName(testAppId, testApiBasePath, consumerClientId, providerClientId)
				l.Status.AppliedPlacement = &spectrev1.AppliedListenerPlacementStatus{
					Fingerprint:        goldenFingerprint,
					CaptureRoute:       &ctypes.ObjectRef{Name: util.MakeRouteName(testApiBasePath), Namespace: listenerZoneStatus},
					CaptureZone:        zoneRef.DeepCopy(),
					CaptureEventStore:  esRef.DeepCopy(),
					DeliveryZone:       zoneRef.DeepCopy(),
					DeliveryEventStore: esRef.DeepCopy(),
					CallbackOriginZone: zoneRef.DeepCopy(),
					CallbackBaseURL:    testCallbackURL,
				}
				l.Status.RouteListener = &ctypes.ObjectRef{Name: rlName, Namespace: listenerZoneStatus}
				rqName := util.MakeSubscriberName(util.MakeBridgeSubscriberId(consumerClientId, testAppId, testApiBasePath, "rq"))
				rpName := util.MakeSubscriberName(util.MakeBridgeSubscriberId(consumerClientId, testAppId, testApiBasePath, "rp"))
				l.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: rqName, Namespace: listenerZoneStatus},
					{Name: rpName, Namespace: listenerZoneStatus},
				}
				labels := map[string]string{
					cconfig.OwnerUidLabelKey:                 "listener-uid-001",
					handler.AuthorizationFingerprintLabelKey: goldenFingerprint,
				}
				rls := []gatewayv1.RouteListener{{ObjectMeta: metav1.ObjectMeta{
					Name: rlName, Namespace: listenerZoneStatus, Labels: labels,
				}}}
				subs := []pubsubv1.Subscriber{
					{ObjectMeta: metav1.ObjectMeta{Name: rqName, Namespace: listenerZoneStatus, Labels: labels}},
					{ObjectMeta: metav1.ObjectMeta{Name: rpName, Namespace: listenerZoneStatus, Labels: labels}},
				}

				for i := range 2 {
					if i > 0 {
						mockEarlyRestrictionGranted(l)
					}
					mockReadyReconcile(rls, subs)
					start := len(fakeClient.Calls)
					next, err := reconcile(l)
					Expect(err).ToNot(HaveOccurred())
					Expect(next.Status.Draining).To(BeNil())
					Expect(countCalls(start, "Delete", nil)).To(BeZero())
					expectGolden(next)
					l = next
				}
			})
		})

		Context("dual-gate: different provider team blocks provisioning", func() {
			It("should block when provider gate is granted but consumer gate is pending", func() {
				listener := newListener()
				// Consumer team matches observer — A==C in this test case.
				consumerApp := makeConsumerApp()
				consumerApp.Spec.Team = "observer-team"

				// Provider team differs from observer — A!=P, so Simple strategy.
				providerApp := makeProviderApp()
				providerApp.Spec.Team = "other-team"

				mockGetConsumerApp(consumerApp)
				mockGetProviderApp(providerApp)
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(consumerApp) // A==C: observer resolves to consumer (same team override)
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()
				mockNoStaleChildren()
				// Provider gate: granted (first in dual-gate order).
				mockApprovalGrantedGate("provider")
				// Consumer gate: pending (second in dual-gate order).
				mockApprovalPendingGate()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				procCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(procCond).ToNot(BeNil())
				Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))
			})
		})

		// --- Brief section 6: Regression coverage from Phase 1 ---

		Context("regression: stale children removed BEFORE approval evaluation", func() {
			It("should delete stale children before evaluating approval", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockListRoutes()

				// Stale RouteListener with old fingerprint.
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
											handler.AuthorizationFingerprintLabelKey: "old-fp",
										},
									},
								},
							},
						}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetName() == "stale-rl"
					}), mock.Anything).
					Return(nil).Once()

				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// After stale removal, approval evaluates (pending stops test here).
				mockApprovalPending()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				procCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(procCond).ToNot(BeNil())
				Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))
			})
		})

		Context("regression: unsupported route mode cleanup happens before approval", func() {
			It("should clean up and block without ever creating ApprovalRequests", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				// No mockGetObserverApp: route mode check blocks before observer resolution.
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
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

				// resolvePublisherNamespace: topology fallback.
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
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				// Route mode rejection happens BEFORE approval.
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pass-through"))

				// No approval-related status refs set.
				Expect(listener.Status.ProviderApproval).To(BeNil())
				Expect(listener.Status.ConsumerApproval).To(BeNil())
				Expect(listener.Status.ProviderApprovalRequest).To(BeNil())
				Expect(listener.Status.ConsumerApprovalRequest).To(BeNil())
			})
		})

		// --- Dual-gate status refs (both approval refs populated) ---

		Context("dual-gate: both approval status refs populated on grant", func() {
			It("should populate all four approval status refs", func() {
				listener := setupFullHappyPath()
				fakeClient.EXPECT().AnyChanged().Return(false).Once()
				fakeClient.EXPECT().AllReady().Return(true).Once()
				mockListenerReadinessChecks()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				Expect(listener.Status.ProviderApproval).ToNot(BeNil())
				Expect(listener.Status.ConsumerApproval).ToNot(BeNil())
				Expect(listener.Status.ProviderApprovalRequest).ToNot(BeNil())
				Expect(listener.Status.ConsumerApprovalRequest).ToNot(BeNil())
			})
		})

		// --- R7: Early restriction check (GPT §1 + §8 regression) ---

		Context("early restriction: provider Approval is Rejected", func() {
			It("should cleanup children without resolving Applications", func() {
				listener := newListener()
				// Pre-populate status refs as if a previous reconcile created them.
				listener.Status.ProviderApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--provider", Namespace: listenerNamespace,
				}
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				}

				// Early restriction: fetch provider Approval — Rejected.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Spec.State = approvalv1.ApprovalStateRejected
					}).
					Return(nil).Once()

				// handleDenialCleanup: deleteAllOwnedChildren.
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

				// resolvePublisherNamespace: status refs cleared — label fallback.
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
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// Status cleared.
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())

				// AccessDenied condition set with early restriction message.
				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(readyCond.Message).To(ContainSubstring("early restriction"))
				Expect(readyCond.Message).To(ContainSubstring("provider"))
			})
		})

		Context("early restriction: consumer Approval is Suspended", func() {
			It("should cleanup via early restriction on Suspended state", func() {
				listener := newListener()
				listener.Status.ConsumerApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--consumer", Namespace: listenerNamespace,
				}

				// Early restriction: skip provider (nil ref), fetch consumer — Suspended.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--consumer", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Spec.State = approvalv1.ApprovalStateSuspended
					}).
					Return(nil).Once()

				// handleDenialCleanup: no children.
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

				// resolvePublisherNamespace: no children, no status refs.
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

				// Topology fallback for resolvePublisherNamespace.
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(readyCond.Message).To(ContainSubstring("consumer"))
			})
		})

		Context("early restriction: Approval is Granted — no early exit", func() {
			It("should continue to normal flow when Approvals are Granted", func() {
				listener := newListener()
				listener.Status.ProviderApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--provider", Namespace: listenerNamespace,
				}
				listener.Status.ConsumerApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--consumer", Namespace: listenerNamespace,
				}

				// Early restriction: both Approvals Granted — no early exit.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Spec.State = approvalv1.ApprovalStateGranted
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--consumer", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Spec.State = approvalv1.ApprovalStateGranted
					}).
					Return(nil).Once()

				// Normal flow continues — consumer App fails to resolve (blocks).
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: consumerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
					Return(fmt.Errorf("not found")).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer Application"))
			})
		})

		Context("early restriction: Approval fetch error — continues to normal flow", func() {
			It("should log the error and continue when Approval Get fails", func() {
				listener := newListener()
				listener.Status.ProviderApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--provider", Namespace: listenerNamespace,
				}

				// Early restriction: Get returns error (not NotFound).
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Return(fmt.Errorf("internal API error")).Once()

				// Normal flow continues — consumer App fails to resolve (blocks).
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: consumerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
					Return(fmt.Errorf("not found")).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer Application"))
			})
		})

		Context("early restriction: Approval NotFound — continues to normal flow", func() {
			It("should continue when Approval is NotFound (pending creation)", func() {
				listener := newListener()
				listener.Status.ProviderApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--provider", Namespace: listenerNamespace,
				}

				// Early restriction: Approval NotFound — skip.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Return(errors.NewNotFound(schema.GroupResource{Group: "approval.cp.ei.telekom.de", Resource: "approvals"}, "")).Once()

				// Normal flow continues.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: consumerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
					Return(fmt.Errorf("not found")).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer Application"))
			})
		})

		Context("early restriction: Expired-from-Suspended triggers cleanup", func() {
			It("should trigger cleanup on Expired state with LastState=Suspended", func() {
				listener := newListener()
				listener.Status.ProviderApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--provider", Namespace: listenerNamespace,
				}

				// Early restriction: Expired with LastState=Suspended.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Spec.State = approvalv1.ApprovalStateExpired
						approval.Status.LastState = approvalv1.ApprovalStateSuspended
					}).
					Return(nil).Once()

				// handleDenialCleanup: no children.
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

				// resolvePublisherNamespace: no children.
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
				mockGetConsumerApp(makeConsumerApp())
				mockGetZone()

				// cleanupGenericPublisherIfOrphaned.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
			})
		})

		Context("early restriction: Expired-from-Granted does NOT trigger cleanup", func() {
			It("should continue to normal flow when Expired but LastState is Granted", func() {
				listener := newListener()
				listener.Status.ProviderApproval = &ctypes.ObjectRef{
					Name: "listener--test-listener--provider", Namespace: listenerNamespace,
				}

				// Early restriction: Expired with LastState=Granted — NOT denied.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "listener--test-listener--provider", Namespace: listenerNamespace},
						mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						approval := out.(*approvalv1.Approval)
						approval.Spec.State = approvalv1.ApprovalStateExpired
						approval.Status.LastState = approvalv1.ApprovalStateGranted
					}).
					Return(nil).Once()

				// Normal flow continues.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: consumerAppName, Namespace: listenerNamespace}, mock.AnythingOfType("*v1.Application")).
					Return(fmt.Errorf("not found")).Once()

				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer Application"))
			})
		})

		Context("early restriction: no approval refs in status — continues normally", func() {
			It("should skip early restriction when no approval refs exist", func() {
				listener := newListener()
				// No approval refs in status — early restriction check is a no-op.

				// Normal flow continues — consumer App fails to resolve (blocks).
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
		// mockDeleteNamespaceResolution stubs the List calls used by
		// resolvePublisherNamespace when status refs are nil (owner-label fallback).
		mockDeleteNamespaceResolution := func(rlItems []gatewayv1.RouteListener, subItems []pubsubv1.Subscriber) {
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{Items: rlItems}
				}).
				Return(nil).Once()
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{Items: subItems}
				}).
				Return(nil).Once()
		}

		// mockDeletePhase1 stubs Phase 1 (RouteListener deletion) when no owned
		// RouteListeners exist beyond the status ref.
		mockDeletePhase1NoRL := func() {
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{}
				}).
				Return(nil).Once()
		}

		// mockDeletePhase2NoSub stubs Phase 2+3 (Subscriber deletion + fresh-list)
		// when no owned Subscribers exist.
		mockDeletePhase2NoSub := func() {
			// Phase 2: list owned subs — empty.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()
			// Phase 3: fresh-list — empty.
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
				}).
				Return(nil).Once()
		}

		// mockDeletePublisherCleanup stubs the Subscriber-usage-based Publisher
		// orphan check (Phase 5). noSubscribers=true means no Sub references the
		// generic Publisher, so it will be deleted.
		mockDeletePublisherCleanup := func(noSubscribers bool) {
			fakeClient.EXPECT().
				List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
					items := []pubsubv1.Subscriber{}
					if !noSubscribers {
						items = append(items, pubsubv1.Subscriber{
							Spec: pubsubv1.SubscriberSpec{
								Publisher: ctypes.ObjectRef{
									Name:      util.MakePublisherName(util.GenericEventType),
									Namespace: listenerZoneStatus,
								},
							},
						})
					}
					*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{Items: items}
				}).
				Return(nil).Once()
		}

		Context("when Subscriber deletion is requested before any Publisher deletion", func() {
			It("should delete RouteListener first, then Subscribers, then check Publisher", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-1", Namespace: listenerZoneStatus}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "sub-rq", Namespace: listenerZoneStatus},
				}

				// Phase 0: namespace from status refs (rl-1 namespace).
				// Phase 1: delete RL from status.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "rl-1", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.RouteListener")).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
					Return(nil).Once()
				mockDeletePhase1NoRL()

				// Phase 2: delete Sub from status.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "sub-rq", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.Subscriber")).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Return(nil).Once()
				mockDeletePhase2NoSub()

				// Phase 5: Publisher cleanup — no subscribers reference it.
				mockDeletePublisherCleanup(true)
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeNil())
			})
		})

		Context("when Publisher is not deleted while a Subscriber still exists", func() {
			It("should return RetryableWithDelayError when owned Subscribers remain", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-1", Namespace: listenerZoneStatus}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "sub-rq", Namespace: listenerZoneStatus},
				}

				// Phase 0: namespace from status.
				// Phase 1: RL deleted.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "rl-1", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.RouteListener")).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
					Return(nil).Once()
				mockDeletePhase1NoRL()

				// Phase 2: delete Sub from status — sent.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "sub-rq", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.Subscriber")).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Return(nil).Once()

				// Phase 2 label-list: empty.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// Phase 3: fresh-list — Subscriber still exists (finalizer running).
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{ObjectMeta: metav1.ObjectMeta{Name: "sub-rq", Namespace: listenerZoneStatus}},
							},
						}
					}).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("waiting for bridge Subscriber finalization"))
			})
		})

		Context("when a deleting Subscriber still keeps the Publisher", func() {
			It("should NOT delete Publisher when a terminating Subscriber references it", func() {
				listener := newListener()
				// Status refs already cleared from prior reconcile; use label fallback.
				mockDeleteNamespaceResolution(nil, []pubsubv1.Subscriber{
					{ObjectMeta: metav1.ObjectMeta{Name: "sub-rq", Namespace: listenerZoneStatus}},
				})

				// Phase 1: no RL.
				mockDeletePhase1NoRL()

				// Phase 2: label-list returns the terminating sub (already deleted, still present).
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						now := metav1.Now()
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name:              "sub-rq",
										Namespace:         listenerZoneStatus,
										DeletionTimestamp: &now,
										Finalizers:        []string{"pubsub.cp.ei.telekom.de/finalizer"},
									},
								},
							},
						}
					}).
					Return(nil).Once()

				// Delete issued.
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Subscriber"), mock.Anything).
					Return(nil).Once()

				// Phase 3: fresh-list — still present.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						now := metav1.Now()
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{
									ObjectMeta: metav1.ObjectMeta{
										Name:              "sub-rq",
										Namespace:         listenerZoneStatus,
										DeletionTimestamp: &now,
										Finalizers:        []string{"pubsub.cp.ei.telekom.de/finalizer"},
									},
								},
							},
						}
					}).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("waiting for bridge Subscriber finalization"))
			})
		})

		Context("when provider-zone namespace is resolved from status/label instead of consumer-zone fallback", func() {
			It("should use status RouteListener namespace for Publisher cleanup", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-1", Namespace: "provider-zone-ns"}

				// Phase 1.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "rl-1", Namespace: "provider-zone-ns"}, mock.AnythingOfType("*v1.RouteListener")).
					Return(errors.NewNotFound(schema.GroupResource{}, "")).Once()
				mockDeletePhase1NoRL()

				// Phase 2+3.
				mockDeletePhase2NoSub()

				// Phase 5: Publisher cleanup in provider-zone-ns (not consumer zone).
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetNamespace() == "provider-zone-ns"
					}), mock.Anything).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when a Listener in another zone does not retain this zone's generic Publisher", func() {
			It("should delete Publisher when no Subscriber in the zone references it", func() {
				listener := newListener()
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "sub-rq", Namespace: listenerZoneStatus},
				}

				// Phase 1.
				mockDeletePhase1NoRL()

				// Phase 2: delete sub.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "sub-rq", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.Subscriber")).
					Return(errors.NewNotFound(schema.GroupResource{}, "")).Once()
				mockDeletePhase2NoSub()

				// Phase 5: List Subscribers in zone — the only Subscriber references a
				// DIFFERENT Publisher (another zone's event type).
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{
									ObjectMeta: metav1.ObjectMeta{Name: "other-sub", Namespace: listenerZoneStatus},
									Spec: pubsubv1.SubscriberSpec{
										Publisher: ctypes.ObjectRef{
											Name:      "different-publisher",
											Namespace: listenerZoneStatus,
										},
									},
								},
							},
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

		Context("when another Subscriber referencing the same generic Publisher retains it", func() {
			It("should NOT delete Publisher when another Subscriber references it", func() {
				listener := newListener()
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "sub-rq", Namespace: listenerZoneStatus},
				}

				// Phase 1.
				mockDeletePhase1NoRL()

				// Phase 2: delete sub.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "sub-rq", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.Subscriber")).
					Return(errors.NewNotFound(schema.GroupResource{}, "")).Once()
				mockDeletePhase2NoSub()

				// Phase 5: another Subscriber still references the generic Publisher.
				mockDeletePublisherCleanup(false)

				err := h.Delete(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when lost status refs are recovered through owner labels", func() {
			It("should delete children found via owner labels when status refs are nil", func() {
				listener := newListener()
				// No status refs — everything must be found via owner labels.

				// Phase 0: resolve namespace from owner-labelled children.
				mockDeleteNamespaceResolution(
					[]gatewayv1.RouteListener{
						{ObjectMeta: metav1.ObjectMeta{Name: "rl-orphan", Namespace: listenerZoneStatus}},
					},
					nil,
				)

				// Phase 1: label-list RouteListeners (re-listed in delete phase).
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*gatewayv1.RouteListenerList) = gatewayv1.RouteListenerList{
							Items: []gatewayv1.RouteListener{
								{ObjectMeta: metav1.ObjectMeta{Name: "rl-orphan", Namespace: listenerZoneStatus}},
							},
						}
					}).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.MatchedBy(func(obj client.Object) bool {
						return obj.GetName() == "rl-orphan"
					}), mock.Anything).
					Return(nil).Once()

				// Phase 2+3: no Subscribers.
				mockDeletePhase2NoSub()

				// Phase 5: Publisher cleanup.
				mockDeletePublisherCleanup(true)
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("retry uses RetryableWithDelayErrorf", func() {
			It("should use RetryableWithDelayErrorf not RequeueAfter", func() {
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "rl-1", Namespace: listenerZoneStatus}

				// Phase 1.
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: "rl-1", Namespace: listenerZoneStatus}, mock.AnythingOfType("*v1.RouteListener")).
					Return(errors.NewNotFound(schema.GroupResource{}, "")).Once()
				mockDeletePhase1NoRL()

				// Phase 2: label-list empty.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{}
					}).
					Return(nil).Once()

				// Phase 3: fresh-list — still has a sub.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						*list.(*pubsubv1.SubscriberList) = pubsubv1.SubscriberList{
							Items: []pubsubv1.Subscriber{
								{ObjectMeta: metav1.ObjectMeta{Name: "lingering-sub", Namespace: listenerZoneStatus}},
							},
						}
					}).
					Return(nil).Once()

				err := h.Delete(ctx, listener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("waiting for bridge Subscriber finalization"))
			})
		})
	})
})
