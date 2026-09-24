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

	// mockGetCaptureRoute stubs the gateway Route lookup of the capture zone.
	mockGetCaptureRoute := func() {
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
	}

	// mockListRoutes stubs the gateway Route lookup and the subsequent
	// ApiExposure list call used by verifyProviderBinding.
	mockListRoutes := func() {
		routeName := util.MakeRouteName(testApiBasePath)
		mockGetCaptureRoute()

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
			obj := 1
			if method == "Get" {
				obj = 2 // Get(ctx, key, obj)
			}
			if sample == nil || reflect.TypeOf(calls[i].Arguments.Get(obj)) == reflect.TypeOf(sample) {
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

	// countNamespacedSubscriberLists counts Subscriber Lists scoped to a
	// namespace since index from: the orphan check behind a Publisher delete.
	// Owner-label inventories are cluster-wide and not counted.
	countNamespacedSubscriberLists := func(from int) int {
		n := 0
		for _, call := range fakeClient.Calls[from:] {
			if call.Method != "List" {
				continue
			}
			if _, ok := call.Arguments.Get(1).(*pubsubv1.SubscriberList); !ok {
				continue
			}
			lo := &client.ListOptions{}
			for _, a := range call.Arguments[2:] {
				if o, ok := a.(client.ListOption); ok {
					o.ApplyToList(lo)
				}
			}
			if lo.Namespace != "" {
				n++
			}
		}
		return n
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

	// earlyRead is the result of one step-0.5 read: the referenced object in
	// state (Granted by default), with uid instead of the ref's UID when set, or
	// the read error err.
	type earlyRead struct {
		state approvalv1.ApprovalState
		uid   k8stypes.UID
		err   error
	}

	// mockEarlyReads stubs the step-0.5 reads of the provider and consumer refs
	// with the result reads names for each gate. set decodes a result into the
	// Get output; reads stop at the first result stop reports as denying.
	// Register them before the gate mocks: testify matches expectations in
	// registration order.
	mockEarlyReads := func(
		refs [2]*ctypes.ObjectRef,
		sample string,
		reads map[string]earlyRead,
		set func(out client.Object, ref *ctypes.ObjectRef, r earlyRead),
		stop func(r earlyRead) bool,
	) {
		for i, key := range []string{"provider", "consumer"} {
			Expect(refs[i]).ToNot(BeNil())
			ref := refs[i].DeepCopy()
			r, ok := reads[key]
			if !ok {
				r.state = approvalv1.ApprovalStateGranted
			}
			call := fakeClient.EXPECT().Get(ctx, ref.K8s(), mock.AnythingOfType(sample))
			if r.err != nil {
				call.Return(r.err).Once()
				continue
			}
			call.Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				set(out, ref, r)
			}).Return(nil).Once()
			if stop(r) {
				return
			}
		}
	}

	// mockEarlyApprovalReads stubs the step-0.5 reads of the Approvals
	// referenced by l's status.
	mockEarlyApprovalReads := func(l *spectrev1.Listener, reads map[string]earlyRead) {
		mockEarlyReads([2]*ctypes.ObjectRef{l.Status.ProviderApproval, l.Status.ConsumerApproval}, "*v1.Approval", reads,
			func(out client.Object, _ *ctypes.ObjectRef, r earlyRead) {
				out.(*approvalv1.Approval).Spec.State = r.state
			},
			func(r earlyRead) bool {
				return r.state == approvalv1.ApprovalStateRejected || r.state == approvalv1.ApprovalStateSuspended
			})
	}

	// mockEarlyApprovalsGranted stubs the step-0.5 reads of the Approvals
	// referenced by l's status, both Granted.
	mockEarlyApprovalsGranted := func(l *spectrev1.Listener) {
		mockEarlyApprovalReads(l, nil)
	}

	// mockEarlyRequestReads stubs the step-0.5 reads of the current
	// ApprovalRequests referenced by l's status, done while capture is applied.
	// A Rejected request with the ref's UID ends the reads.
	mockEarlyRequestReads := func(l *spectrev1.Listener, reads map[string]earlyRead) {
		mockEarlyReads([2]*ctypes.ObjectRef{l.Status.ProviderApprovalRequest, l.Status.ConsumerApprovalRequest}, "*v1.ApprovalRequest", reads,
			func(out client.Object, ref *ctypes.ObjectRef, r earlyRead) {
				req := out.(*approvalv1.ApprovalRequest)
				req.Name, req.Namespace, req.UID = ref.Name, ref.Namespace, ref.UID
				if r.uid != "" {
					req.UID = r.uid
				}
				req.Spec.State = r.state
			},
			func(r earlyRead) bool { return r.state == approvalv1.ApprovalStateRejected && r.uid == "" })
	}

	// captureApplied mirrors when step 0.5 reads the request refs: no drain is
	// active and a fingerprint or capture child ref is recorded.
	captureApplied := func(l *spectrev1.Listener) bool {
		ap := l.Status.AppliedPlacement
		return l.Status.Draining == nil &&
			((ap != nil && ap.Fingerprint != "") || l.Status.RouteListener != nil || len(l.Status.EventSubscriptions) > 0)
	}

	// mockEarlyRestrictionGranted stubs every step-0.5 read for l: both Approvals
	// Granted and, while capture is applied, both current requests not rejected.
	mockEarlyRestrictionGranted := func(l *spectrev1.Listener) {
		mockEarlyApprovalsGranted(l)
		if captureApplied(l) {
			mockEarlyRequestReads(l, nil)
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
	// fingerprint label so the stale-child check keeps them.
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

	// driveDrainToCleaningPublisher drives a persisted Stopping checkpoint for
	// the live RouteListener rl and Subscribers subs through the next four
	// reconciles: the RouteListener is deleted with its UID+RV preconditions
	// and waited for before any Subscriber is touched, then the Subscribers
	// likewise. It returns the Listener at CleaningPublisher.
	driveDrainToCleaningPublisher := func(l *spectrev1.Listener, rl gatewayv1.RouteListener, subs []pubsubv1.Subscriber) *spectrev1.Listener {
		rlKey := k8stypes.NamespacedName{Name: rl.Name, Namespace: rl.Namespace}
		// R2: Stopping deletes the old RouteListener with UID+RV preconditions.
		fakeClient.EXPECT().
			Get(ctx, rlKey, mock.AnythingOfType("*v1.RouteListener")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				rl.DeepCopyInto(out.(*gatewayv1.RouteListener))
			}).
			Return(nil).Once()
		fakeClient.EXPECT().
			Delete(ctx, mock.AnythingOfType("*v1.RouteListener"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, opts ...client.DeleteOption) {
				do := (&client.DeleteOptions{}).ApplyOptions(opts)
				Expect(do.Preconditions).ToNot(BeNil())
				Expect(do.Preconditions.UID).To(HaveValue(Equal(rl.UID)))
				Expect(do.Preconditions.ResourceVersion).To(HaveValue(Equal(rl.ResourceVersion)))
			}).
			Return(nil).Once()
		start := len(fakeClient.Calls)
		l, err := reconcile(l)
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
			Return(errors.NewNotFound(schema.GroupResource{Group: gatewayv1.GroupVersion.Group, Resource: "routelisteners"}, rl.Name)).Once()
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(countCalls(start, "Delete", nil)).To(BeZero())
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))
		Expect(l.Status.RouteListener).To(BeNil())
		Expect(l.Status.EventSubscriptions).To(HaveLen(len(subs)))

		// R4: DrainingSubscribers deletes both Subscribers with UID+RV preconditions.
		subRVs := map[k8stypes.UID]string{}
		for i := range subs {
			subRVs[subs[i].UID] = subs[i].ResourceVersion
			sub := subs[i].DeepCopy()
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
				Expect(subRVs).To(HaveKey(obj.GetUID()))
				Expect(do.Preconditions).ToNot(BeNil())
				Expect(do.Preconditions.UID).To(HaveValue(Equal(obj.GetUID())))
				Expect(do.Preconditions.ResourceVersion).To(HaveValue(Equal(subRVs[obj.GetUID()])))
			}).
			Return(nil).Times(len(subs))
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(Equal(len(subs)))
		Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
		Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(BeZero())
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))

		// R5: both Subscribers are gone.
		for i := range subs {
			fakeClient.EXPECT().
				Get(ctx, k8stypes.NamespacedName{Name: subs[i].Name, Namespace: subs[i].Namespace}, mock.AnythingOfType("*v1.Subscriber")).
				Return(errors.NewNotFound(schema.GroupResource{Group: pubsubv1.GroupVersion.Group, Resource: "subscribers"}, subs[i].Name)).Once()
		}
		start = len(fakeClient.Calls)
		l, err = reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(countCalls(start, "Delete", nil)).To(BeZero())
		Expect(l.Status.Draining).ToNot(BeNil())
		Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))
		Expect(l.Status.EventSubscriptions).To(BeEmpty())

		return l
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

		// R1: the gate's current request is Rejected. The early read of that
		// request fails, so only the dual-gate build (step 7) reports the
		// rejection. The drain checkpoint is written and the reconcile returns
		// before anything is deleted.
		preR1 := f.listener
		mockR1 := func() {
			mockEarlyApprovalsGranted(preR1)
			mockEarlyRequestReads(preR1, map[string]earlyRead{gate: {err: errors.NewServiceUnavailable("etcd leader changed")}})
			mockResolveTopology()
			mockOwnedLists(liveRLs, f.subs) // stale-child check: same fingerprint, kept
			mockRejectedGates(gate)
			mockOwnedLists(liveRLs, f.subs) // drainCapture inventory
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

		l = driveDrainToCleaningPublisher(l, f.rl, f.subs)

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
		mockOwnedLists(nil, nil) // stale-child check
		mockRejectedGates(gate)
		mockOwnedLists(nil, nil) // drainCapture inventory: nothing left
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
	// Each one takes one owner-label inventory, finds nothing and touches no
	// Publisher: the shared Publisher is only cleaned by a drain.
	expectStaysDrainedWhileRejected := func(l *spectrev1.Listener, gate string) {
		for range 3 {
			mockEarlyRestrictionGranted(l)
			mockResolveTopology()
			mockOwnedLists(nil, nil) // stale-child check
			mockRejectedGates(gate)
			mockOwnedLists(nil, nil) // drainCapture inventory

			start := len(fakeClient.Calls)
			next, err := reconcile(l)
			Expect(err).ToNot(HaveOccurred())
			Expect(next.Status.Draining).To(BeNil())
			Expect(next.Status.AppliedPlacement).To(BeNil())
			Expect(next.Status.RouteListener).To(BeNil())
			Expect(next.Status.EventSubscriptions).To(BeEmpty())
			Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
			Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
			Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(BeZero())
			Expect(countNamespacedSubscriberLists(start)).To(BeZero())
			expectNoCaptureCreates(start)
			expectRequestDenied(next, gate)
			l = next
		}
	}

	// --- Stale-generation and legacy-migration helpers ---

	// ownedChildMeta is the metadata of a live child owned by the test Listener
	// in its zone, labelled with fingerprint; an empty fingerprint leaves it
	// unlabelled like a child of the prior authorization policy.
	ownedChildMeta := func(name, fingerprint string) metav1.ObjectMeta {
		labels := map[string]string{cconfig.OwnerUidLabelKey: "listener-uid-001"}
		if fingerprint != "" {
			labels[handler.AuthorizationFingerprintLabelKey] = fingerprint
		}
		return metav1.ObjectMeta{
			Name:            name,
			Namespace:       listenerZoneStatus,
			UID:             k8stypes.UID(name + "-uid"),
			ResourceVersion: "5",
			Labels:          labels,
		}
	}

	// staleCapture is the live capture of an earlier generation.
	type staleCapture struct {
		rl   gatewayv1.RouteListener
		subs []pubsubv1.Subscriber
	}

	// withStaleCapture gives l a live RouteListener labelled rlFingerprint and
	// one Subscriber per subFingerprints entry, all referenced by l's status.
	withStaleCapture := func(l *spectrev1.Listener, rlFingerprint string, subFingerprints ...string) *staleCapture {
		s := &staleCapture{rl: gatewayv1.RouteListener{ObjectMeta: ownedChildMeta("stale-rl", rlFingerprint)}}
		l.Status.RouteListener = ctypes.ObjectRefFromObject(&s.rl)
		for i, fp := range subFingerprints {
			s.subs = append(s.subs, pubsubv1.Subscriber{ObjectMeta: ownedChildMeta(fmt.Sprintf("stale-sub-%d", i), fp)})
			l.Status.EventSubscriptions = append(l.Status.EventSubscriptions, *ctypes.ObjectRefFromObject(&s.subs[i]))
		}
		return s
	}

	// expectStaleCheckpoint asserts that the reconcile since call index from only
	// wrote a Stopping checkpoint for s: nothing was deleted, and no child,
	// Publisher or ApprovalRequest was created.
	expectStaleCheckpoint := func(l *spectrev1.Listener, s *staleCapture, from int) {
		d := l.Status.Draining
		Expect(d).ToNot(BeNil())
		Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
		Expect(d.Reason).To(Equal("stale authorization generation"))
		Expect(d.OldFingerprint).To(BeEmpty())
		Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&s.rl)}))
		want := make([]ctypes.ObjectRef, len(s.subs))
		for i := range s.subs {
			want[i] = *ctypes.ObjectRefFromObject(&s.subs[i])
		}
		Expect(d.OldSubscribers).To(Equal(want))
		Expect(countCalls(from, "Delete", nil)).To(BeZero())
		Expect(countCalls(from, "CreateOrUpdate", nil)).To(BeZero())
	}

	// expectOrphanedPublisherDelete stubs the CleaningPublisher orphan check in
	// the zone namespace (no Subscriber left) and the generic Publisher Delete
	// there. Register it before owner-label List stubs: those match any List.
	expectOrphanedPublisherDelete := func() {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.SubscriberList"), client.InNamespace(listenerZoneStatus)).
			Return(nil).Once()
		fakeClient.EXPECT().
			Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, _ ...client.DeleteOption) {
				Expect(obj.GetName()).To(Equal(util.MakePublisherName(util.GenericEventType)))
				Expect(obj.GetNamespace()).To(Equal(listenerZoneStatus))
			}).
			Return(nil).Once()
	}

	// finishStaleDrain drives the persisted checkpoint for s through the
	// deletions and the CleaningPublisher reconcile, which removes the orphaned
	// generic Publisher, completes the drain and runs the rest of the reconcile
	// that mockRest stubs. It returns the Listener and the call index at which
	// that reconcile started.
	finishStaleDrain := func(l *spectrev1.Listener, s *staleCapture, mockRest func()) (*spectrev1.Listener, int) {
		l = driveDrainToCleaningPublisher(l, s.rl, s.subs)
		expectOrphanedPublisherDelete()
		mockRest()
		start := len(fakeClient.Calls)
		l, err := reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		Expect(l.Status.Draining).To(BeNil())
		Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1))
		Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
		Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
		expectNoCaptureCreates(start)
		return l, start
	}

	// expectStaleDrainedBeforeApproval reconciles the v2 Listener l whose status
	// references the stale capture s: R1 only checkpoints, R2-R5 delete the
	// RouteListener and then the Subscribers with UID+RV preconditions, and only
	// the reconcile completing the drain evaluates the approvals (Pending). No
	// capture child or Publisher is created throughout.
	expectStaleDrainedBeforeApproval := func(l *spectrev1.Listener, s *staleCapture) {
		mockGetZone()
		mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
		mockResolveTopology()
		mockOwnedLists([]gatewayv1.RouteListener{s.rl}, s.subs) // stale-child check
		first := len(fakeClient.Calls)
		l, err := reconcile(l)
		Expect(err).ToNot(HaveOccurred())
		expectStaleCheckpoint(l, s, first)

		l, last := finishStaleDrain(l, s, func() {
			mockResolveTopology()
			mockOwnedLists(nil, nil) // stale-child check: nothing left
			mockApprovalPending()
		})
		// The approvals are evaluated only now, after the drain.
		Expect(countCalls(first, "CreateOrUpdate", &approvalv1.ApprovalRequest{})).To(Equal(2))
		Expect(countCalls(last, "CreateOrUpdate", &approvalv1.ApprovalRequest{})).To(Equal(2))
		expectNoCaptureCreates(first)
		procCond := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeProcessing)
		Expect(procCond).ToNot(BeNil())
		Expect(procCond.Reason).To(Equal(condition.ReasonBlocked))
	}

	legacyApprovalKey := k8stypes.NamespacedName{
		Name:      approvalv1.ApprovalName("Listener", listenerName),
		Namespace: listenerNamespace,
	}

	// mockLegacyApprovalGets stubs n reads of the legacy unscoped Approval by its
	// deterministic name, returning approval, or NotFound when it is nil.
	// Register them before gate stubs: those match every Approval key.
	mockLegacyApprovalGets := func(approval *approvalv1.Approval, n int) {
		call := fakeClient.EXPECT().Get(ctx, legacyApprovalKey, mock.AnythingOfType("*v1.Approval"))
		if approval == nil {
			call.Return(errors.NewNotFound(schema.GroupResource{Group: approvalv1.GroupVersion.Group, Resource: "approvals"},
				legacyApprovalKey.Name)).Times(n)
			return
		}
		call.Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
			approval.DeepCopyInto(out.(*approvalv1.Approval))
		}).Return(nil).Times(n)
	}

	// mockLegacyRequestGets stubs n reads of the legacy ApprovalRequest ar.
	mockLegacyRequestGets := func(ar *approvalv1.ApprovalRequest, n int) {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: ar.Name, Namespace: ar.Namespace}, mock.AnythingOfType("*v1.ApprovalRequest")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				ar.DeepCopyInto(out.(*approvalv1.ApprovalRequest))
			}).
			Return(nil).Times(n)
	}

	// expectRequestRetirementPrepared asserts the migration is retiring the
	// legacy request ar and has only checkpointed its deletion.
	expectRequestRetirementPrepared := func(l *spectrev1.Listener, ar *approvalv1.ApprovalRequest, from int) {
		m := l.Status.AuthorizationMigration
		Expect(m).ToNot(BeNil())
		Expect(m.Phase).To(Equal("RetiringRequests"))
		Expect(m.RetirementCheckpoint).ToNot(BeNil())
		Expect(m.RetirementCheckpoint.PendingDeletions).To(HaveLen(1))
		pd := m.RetirementCheckpoint.PendingDeletions[0]
		Expect(pd.Kind).To(Equal("ApprovalRequest"))
		Expect(pd.Name).To(Equal(ar.Name))
		Expect(pd.UID).To(Equal(string(ar.UID)))
		Expect(pd.Phase).To(Equal(handler.ExportPendingDeletionPhasePrepared))
		Expect(countCalls(from, "Delete", &approvalv1.ApprovalRequest{})).To(BeZero())
		Expect(l.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"))
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
			// provisionForDenial provisions a Listener whose applied capture the early
			// check cannot see as denied, so the denial is only reported by the
			// dual-gate build (outcomeDenied).
			provisionForDenial := func() *rejectionFixture {
				f := provisionForRejection()
				f.listener.Status.ProviderApproval = nil
				f.listener.Status.ConsumerApproval = nil
				return f
			}

			// expectDeniedCheckpoint asserts the R1 contract: the drain checkpoint
			// is recorded and nothing is deleted or created.
			expectDeniedCheckpoint := func(f *rejectionFixture, l *spectrev1.Listener, from int) {
				d := l.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(d.Reason).To(Equal("approval denied"))
				Expect(d.OldFingerprint).To(Equal(f.fingerprint))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&f.rl)}))
				Expect(d.OldSubscribers).To(ConsistOf(
					*ctypes.ObjectRefFromObject(&f.subs[0]),
					*ctypes.ObjectRefFromObject(&f.subs[1]),
				))
				Expect(d.SourcePublisher).To(Equal(l.Status.AppliedPlacement.Publisher))
				Expect(l.Status.RouteListener).ToNot(BeNil())
				Expect(l.Status.EventSubscriptions).To(HaveLen(2))
				Expect(countCalls(from, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(from)
				ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready).ToNot(BeNil())
				Expect(ready.Reason).To(Equal(condition.ReasonAccessDenied))
			}

			// drainDenied drives the persisted checkpoint to completion (R2-R6).
			// The last reconcile cleans the orphaned Publisher, then the early check
			// now sees the Rejected provider Approval and clears the drained
			// placement without starting another drain.
			drainDenied := func(f *rejectionFixture, l *spectrev1.Listener) {
				l = driveDrainToCleaningPublisher(l, f.rl, f.subs)

				src := l.Status.Draining.SourcePublisher.DeepCopy()
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
				fakeClient.EXPECT().
					Get(ctx, l.Status.ProviderApproval.K8s(), mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						out.(*approvalv1.Approval).Spec.State = approvalv1.ApprovalStateRejected
					}).
					Return(nil).Once()
				mockOwnedLists(nil, nil) // drainCapture inventory: nothing left
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(l.Status.Draining).To(BeNil())
				Expect(l.Status.AppliedPlacement).To(BeNil())
				Expect(l.Status.RouteListener).To(BeNil())
				Expect(l.Status.EventSubscriptions).To(BeEmpty())
				Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1))
				Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
				Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
				expectNoCaptureCreates(start)
			}

			It("should checkpoint first, then drain the RouteListener, the Subscribers and the Publisher", func() {
				f := provisionForDenial()
				liveRLs := []gatewayv1.RouteListener{f.rl}

				// R1: both gates report the Approval Rejected.
				mockEarlyRequestReads(f.listener, nil)
				mockResolveTopology()
				mockOwnedLists(liveRLs, f.subs) // stale-child check: same fingerprint, kept
				mockApprovalDenied()
				mockOwnedLists(liveRLs, f.subs) // drainCapture inventory
				start := len(fakeClient.Calls)
				l, err := reconcile(f.listener)
				Expect(err).ToNot(HaveOccurred())
				expectDeniedCheckpoint(f, l, start)

				drainDenied(f, l)
			})

			It("should checkpoint first when the early check could not read the Approval", func() {
				f := provisionForRejection()
				liveRLs := []gatewayv1.RouteListener{f.rl}

				// R1: the early read of the provider Approval fails, so the denial is
				// only reported by the dual-gate build.
				fakeClient.EXPECT().
					Get(ctx, f.listener.Status.ProviderApproval.K8s(), mock.AnythingOfType("*v1.Approval")).
					Return(errors.NewServiceUnavailable("etcd leader changed")).Once()
				fakeClient.EXPECT().
					Get(ctx, f.listener.Status.ConsumerApproval.K8s(), mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						out.(*approvalv1.Approval).Spec.State = approvalv1.ApprovalStateGranted
					}).
					Return(nil).Once()
				mockEarlyRequestReads(f.listener, nil)
				mockResolveTopology()
				mockOwnedLists(liveRLs, f.subs) // stale-child check: same fingerprint, kept
				mockApprovalDenied()
				mockOwnedLists(liveRLs, f.subs) // drainCapture inventory
				start := len(fakeClient.Calls)
				l, err := reconcile(f.listener)
				Expect(err).ToNot(HaveOccurred())
				expectDeniedCheckpoint(f, l, start)

				drainDenied(f, l)
			})
		})

		Context("when ApprovalRequest is denied (RequestDenied)", func() {
			It("should set AccessDenied naming both gates and touch no capture child or Publisher when nothing is applied", func() {
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
				mockOwnedLists(nil, nil) // drainCapture inventory: nothing to drain

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
				Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(BeZero())
				Expect(countNamespacedSubscriberLists(start)).To(BeZero())
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
			It("should drain old-fingerprint children through the checkpoint before evaluating the replacement grant", func() {
				l := newListener()
				s := withStaleCapture(l, "old-fingerprint", "old-fingerprint")
				expectStaleDrainedBeforeApproval(l, s)
			})

			It("should not delete a same-name child recreated with another UID after the checkpoint", func() {
				l := newListener()
				s := withStaleCapture(l, "old-fingerprint", "old-fingerprint")
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockResolveTopology()
				mockOwnedLists([]gatewayv1.RouteListener{s.rl}, s.subs)
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				expectStaleCheckpoint(l, s, start)

				// R2: the RouteListener was recreated under the same name with another
				// UID: the recorded instance is gone and the replacement is not deleted.
				rl := s.rl.DeepCopy()
				rl.UID = "replacement-rl-uid"
				rl.ResourceVersion = "9"
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: rl.Name, Namespace: rl.Namespace}, mock.AnythingOfType("*v1.RouteListener")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						rl.DeepCopyInto(out.(*gatewayv1.RouteListener))
					}).
					Return(nil).Once()
				start = len(fakeClient.Calls)
				l, err = reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseDrainingSubscribers))

				// R3: the same for the Subscriber, which references the zone's generic
				// Publisher.
				sub := s.subs[0].DeepCopy()
				sub.UID = "replacement-sub-uid"
				sub.ResourceVersion = "9"
				sub.Spec.Publisher = ctypes.ObjectRef{Name: util.MakePublisherName(util.GenericEventType), Namespace: listenerZoneStatus}
				fakeClient.EXPECT().
					Get(ctx, k8stypes.NamespacedName{Name: sub.Name, Namespace: sub.Namespace}, mock.AnythingOfType("*v1.Subscriber")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						sub.DeepCopyInto(out.(*pubsubv1.Subscriber))
					}).
					Return(nil).Once()
				start = len(fakeClient.Calls)
				l, err = reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(l.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseCleaningPublisher))

				// R4: the replacement Subscriber keeps the Publisher. The drain
				// completes, and the replacements, still owned and stale, are recorded
				// by their own checkpoint instead of being adopted; nothing is deleted.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), client.InNamespace(listenerZoneStatus)).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						list.(*pubsubv1.SubscriberList).Items = []pubsubv1.Subscriber{*sub.DeepCopy()}
					}).
					Return(nil).Once()
				mockResolveTopology()
				mockOwnedLists([]gatewayv1.RouteListener{*rl}, []pubsubv1.Subscriber{*sub})
				start = len(fakeClient.Calls)
				l, err = reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
				d := l.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(rl)}))
				Expect(d.OldSubscribers).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(sub)}))
			})
		})

		Context("legacy children without a fingerprint", func() {
			It("should drain unlabelled legacy children through the checkpoint (fail-closed migration)", func() {
				l := newListener()
				s := withStaleCapture(l, "", "")
				expectStaleDrainedBeforeApproval(l, s)
			})
		})

		// A Listener of the prior policy: the freshness decision and the migration
		// record come before anything can remove a child, so draining its children
		// never makes it look fresh.
		Context("legacy migration", func() {
			// legacyListener is a Listener reconciled for the first time after the
			// upgrade: no policy version, no applied placement.
			legacyListener := func() *spectrev1.Listener {
				l := newListener()
				l.Status.AuthorizationPolicyVersion = ""
				return l
			}

			It("records the migration with the drain checkpoint without a legacy Approval and never turns fresh", func() {
				l := legacyListener()
				s := withStaleCapture(l, "", "")

				// R1: isFreshInstall finds no legacy Approval and stops at the unlabelled
				// RouteListener; the record re-reads the Approval; the stale-child check
				// checkpoints the drain.
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockLegacyApprovalGets(nil, 2)
				mockResolveTopology()
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.RouteListenerList"), mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						list.(*gatewayv1.RouteListenerList).Items = []gatewayv1.RouteListener{*s.rl.DeepCopy()}
					}).
					Return(nil).Once() // isFreshInstall
				mockOwnedLists([]gatewayv1.RouteListener{s.rl}, s.subs) // stale-child check
				first := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(l.Status.AuthorizationMigration).ToNot(BeNil())
				Expect(l.Status.AuthorizationMigration.Phase).To(Equal("Blocked"))
				Expect(l.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"))
				expectStaleCheckpoint(l, s, first)

				// R2-R6: the drain. The reconcile completing it finds no child left, yet
				// the persisted record keeps it off the fresh path: the scoped gates are
				// requested (Pending) and v2 is not set.
				l, _ = finishStaleDrain(l, s, func() {
					mockLegacyApprovalGets(nil, 1) // migration discovery
					mockResolveTopology()
					mockOwnedLists(nil, nil) // stale-child check
					mockApprovalPending()
				})
				Expect(l.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"))
				Expect(l.Status.AuthorizationMigration).ToNot(BeNil())

				// R7: the next reconcile, children gone, still does not set v2.
				notFound := errors.NewNotFound(schema.GroupResource{Group: approvalv1.GroupVersion.Group, Resource: "approvals"}, "")
				for _, ref := range []*ctypes.ObjectRef{l.Status.ProviderApproval, l.Status.ConsumerApproval} {
					if ref != nil {
						fakeClient.EXPECT().Get(ctx, ref.K8s(), mock.AnythingOfType("*v1.Approval")).Return(notFound).Once()
					}
				}
				mockLegacyApprovalGets(nil, 1)
				mockResolveTopology()
				mockOwnedLists(nil, nil)
				mockApprovalPending()
				start := len(fakeClient.Calls)
				l, err = reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(l.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"))
				Expect(l.Status.AuthorizationMigration).ToNot(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(first)
			})

			It("drains unlabelled capture first, then retires without an empty migration drain", func() {
				l := legacyListener()
				legacy := makeLegacyApproval(l, approvalv1.ApprovalStateGranted)
				legacyReq := makeLegacyRequest(l)
				l.Status.ProviderApproval = ctypes.ObjectRefFromObject(legacy)
				s := withStaleCapture(l, "", "")

				// R1: the early check reads the Granted legacy Approval; its unscoped
				// ref makes the Listener not fresh; the record reads the Approval and
				// its request; the stale-child check checkpoints the drain.
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockLegacyApprovalGets(legacy, 2)
				mockLegacyRequestGets(legacyReq, 1)
				mockResolveTopology()
				mockOwnedLists([]gatewayv1.RouteListener{s.rl}, s.subs) // stale-child check
				first := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				m := l.Status.AuthorizationMigration
				Expect(m).ToNot(BeNil())
				Expect(m.Phase).To(Equal("Recorded"))
				Expect(m.LegacyApproval).To(Equal(ctypes.ObjectRefFromObject(legacy)))
				Expect(m.LegacyRequests).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(legacyReq)}))
				expectStaleCheckpoint(l, s, first)

				// R2-R6: the drain. The reconcile completing it finds no child left,
				// both scoped gates grant, and the migration's drain step finds nothing
				// to drain: no empty drain starts, and retirement only checkpoints the
				// legacy request's deletion.
				l, last := finishStaleDrain(l, s, func() {
					mockLegacyApprovalGets(legacy, 2)   // early check, migration discovery
					mockLegacyRequestGets(legacyReq, 2) // migration discovery, retirement
					mockResolveTopology()
					mockOwnedLists(nil, nil) // stale-child check
					mockApprovalGranted()
					mockOwnedLists(nil, nil) // migration drain inventory
				})
				Expect(l.Status.AuthorizationMigration.DrainStarted).To(BeFalse())
				expectRequestRetirementPrepared(l, legacyReq, last)
				expectNoCaptureCreates(first)
			})

			// awaitingScoped is a prior-policy Listener whose recorded migration waits
			// in AwaitingScoped: legacy Approval Granted, scoped refs written by an
			// earlier ensureApprovals, and unlabelled prior-policy capture.
			awaitingScoped := func() (*spectrev1.Listener, *approvalv1.Approval, *approvalv1.ApprovalRequest, *staleCapture) {
				l := legacyListener()
				legacy := makeLegacyApproval(l, approvalv1.ApprovalStateGranted)
				legacyReq := makeLegacyRequest(l)
				l.Status.AuthorizationMigration = &spectrev1.AuthorizationMigrationStatus{
					TargetPolicyVersion: "v2",
					Phase:               "AwaitingScoped",
					LegacyApproval:      ctypes.ObjectRefFromObject(legacy),
					LegacyRequests:      []ctypes.ObjectRef{*ctypes.ObjectRefFromObject(legacyReq)},
				}
				l.Status.ProviderApproval = &ctypes.ObjectRef{Name: "ag-v1-provider", Namespace: listenerNamespace}
				l.Status.ConsumerApproval = &ctypes.ObjectRef{Name: "ag-v1-consumer", Namespace: listenerNamespace}
				l.Status.ProviderApprovalRequest = &ctypes.ObjectRef{Name: "ar-v1-provider", Namespace: listenerNamespace, UID: "provider-req-uid"}
				l.Status.ConsumerApprovalRequest = &ctypes.ObjectRef{Name: "ar-v1-consumer", Namespace: listenerNamespace, UID: "consumer-req-uid"}
				return l, legacy, legacyReq, withStaleCapture(l, "", "")
			}

			// expectAwaitingWithoutCapture asserts that l keeps waiting for the scoped
			// gates with no capture and no drain left.
			expectAwaitingWithoutCapture := func(l *spectrev1.Listener) {
				Expect(l.Status.Draining).To(BeNil())
				Expect(l.Status.RouteListener).To(BeNil())
				Expect(l.Status.EventSubscriptions).To(BeEmpty())
				Expect(l.Status.AuthorizationMigration).ToNot(BeNil())
				Expect(l.Status.AuthorizationMigration.Phase).To(Equal("AwaitingScoped"))
				Expect(l.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"))
			}

			// mockAwaitingReconcile stubs the rest of a reconcile whose gates
			// (mockGates) leave the migration waiting: topology, an empty
			// stale-child check and the migration's legacy discovery.
			mockAwaitingReconcile := func(legacy *approvalv1.Approval, legacyReq *approvalv1.ApprovalRequest, mockGates func()) {
				mockLegacyApprovalGets(legacy, 1)   // migration discovery
				mockLegacyRequestGets(legacyReq, 1) // migration discovery
				mockResolveTopology()
				mockOwnedLists(nil, nil) // stale-child check: nothing left
				mockGates()
			}

			// drainWhileAwaiting runs R1, whose early reads (mockR1Early) miss the
			// denial so step 5.10 checkpoints the drain of the prior-policy capture
			// s, then R2-R6: the drain, and the reconcile completing it, whose rest
			// mockR6 stubs. Nothing is created throughout.
			drainWhileAwaiting := func(l *spectrev1.Listener, s *staleCapture, mockR1Early, mockR6 func()) *spectrev1.Listener {
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockR1Early()
				mockResolveTopology()
				mockOwnedLists([]gatewayv1.RouteListener{s.rl}, s.subs) // stale-child check
				first := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				expectStaleCheckpoint(l, s, first)
				Expect(l.Status.AuthorizationMigration.Phase).To(Equal("AwaitingScoped"))

				l, _ = finishStaleDrain(l, s, mockR6)
				expectAwaitingWithoutCapture(l)
				expectNoCaptureCreates(first)
				return l
			}

			It("drains prior-policy capture while waiting on a rejected current scoped request", func() {
				l, legacy, legacyReq, s := awaitingScoped()
				pre := l
				mockRejected := func() { mockRejectedGates("provider") }
				l = drainWhileAwaiting(l, s,
					func() {
						// The early read of the provider request fails.
						mockEarlyApprovalsGranted(pre)
						mockEarlyRequestReads(pre, map[string]earlyRead{"provider": {err: errors.NewServiceUnavailable("etcd leader changed")}})
					},
					func() {
						mockEarlyApprovalsGranted(pre)
						mockAwaitingReconcile(legacy, legacyReq, mockRejected)
					})
				expectRequestDenied(l, "provider")

				// R7: the same rejection keeps the migration waiting without capture.
				mockEarlyApprovalsGranted(l)
				mockAwaitingReconcile(legacy, legacyReq, mockRejected)
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				expectAwaitingWithoutCapture(l)
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
				expectRequestDenied(l, "provider")
			})

			It("drains prior-policy capture while waiting on a denied scoped Approval", func() {
				l, legacy, legacyReq, s := awaitingScoped()
				// No Approval ref is recorded, so the early check cannot see the denial.
				l.Status.ProviderApproval = nil
				l.Status.ConsumerApproval = nil
				pre := l
				l = drainWhileAwaiting(l, s,
					func() { mockEarlyRequestReads(pre, nil) },
					func() {
						mockAwaitingReconcile(legacy, legacyReq, func() {
							mockApprovalDeniedGate("provider")
							mockApprovalGrantedGate("consumer")
						})
					})
				ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready).ToNot(BeNil())
				Expect(ready.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(ready.Message).To(Equal("Approval has been denied (provider gate)"))

				// R7: the scoped Approval refs are recorded now, so the early check
				// sees the denial; nothing is left to drain.
				mockEarlyApprovalReads(l, map[string]earlyRead{"provider": {state: approvalv1.ApprovalStateRejected}})
				mockOwnedLists(nil, nil) // drainCapture inventory: nothing left
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				expectAwaitingWithoutCapture(l)
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
				ready = meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready.Message).To(Equal("Approval has been revoked (provider gate, early restriction)"))
			})

			It("records the migration before the early check drains on a rejected legacy request", func() {
				l := legacyListener()
				legacy := makeLegacyApproval(l, approvalv1.ApprovalStateGranted)
				legacyReq := makeLegacyRequest(l)
				rejected := legacyReq.DeepCopy()
				rejected.Spec.State = approvalv1.ApprovalStateRejected
				l.Status.ProviderApproval = ctypes.ObjectRefFromObject(legacy)
				l.Status.ProviderApprovalRequest = ctypes.ObjectRefFromObject(legacyReq)
				s := withStaleCapture(l, "", "")

				// R1, with C NotReady: the early check reads the Granted legacy
				// Approval and the Rejected legacy request. The freshness decision
				// records the migration before the drain is checkpointed; no topology
				// is read.
				fakeClient.EXPECT().
					Get(ctx, legacyApprovalKey, mock.AnythingOfType("*v1.Approval")).
					Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
						legacy.DeepCopyInto(out.(*approvalv1.Approval))
					}).
					Return(nil).Times(2) // early check, record (the unscoped ref makes it not fresh)
				mockLegacyRequestGets(rejected, 2)                      // early check, record
				mockOwnedLists([]gatewayv1.RouteListener{s.rl}, s.subs) // drainCapture inventory
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				m := l.Status.AuthorizationMigration
				Expect(m).ToNot(BeNil())
				Expect(m.Phase).To(Equal("Recorded"))
				Expect(m.LegacyApproval).To(Equal(ctypes.ObjectRefFromObject(legacy)))
				Expect(l.Status.AuthorizationPolicyVersion).ToNot(Equal("v2"))
				d := l.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Reason).To(Equal("approval request rejected (provider gate, early restriction)"))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&s.rl)}))
				Expect(countCalls(start, "Get", &applicationv1.Application{})).To(BeZero())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countCalls(start, "CreateOrUpdate", nil)).To(BeZero())
			})

			It("does not restart the migration drain it completed when a scoped Approval was revoked meanwhile", func() {
				f := provisionForRejection()
				l := f.listener
				l.Status.AuthorizationPolicyVersion = ""
				legacy := makeLegacyApproval(l, approvalv1.ApprovalStateGranted)
				legacyReq := makeLegacyRequest(l)
				liveRLs := []gatewayv1.RouteListener{f.rl}

				// R1: the migration checkpoints the drain of the applied capture.
				mockEarlyRestrictionGranted(l)
				mockLegacyApprovalGets(legacy, 3)   // isFreshInstall, record, migration discovery
				mockLegacyRequestGets(legacyReq, 2) // record, migration discovery
				mockResolveTopology()
				mockOwnedLists(liveRLs, f.subs) // stale-child check: current fingerprint, kept
				mockApprovalGranted()
				mockOwnedLists(liveRLs, f.subs) // migration drain inventory
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(l.Status.AuthorizationMigration.Phase).To(Equal(handler.ExportMigrationPhaseDraining))
				Expect(l.Status.Draining).ToNot(BeNil())

				// R2-R5: continueDrain deletes the RouteListener, then the Subscribers.
				l = driveDrainToCleaningPublisher(l, f.rl, f.subs)

				// R6-R7: the drain completes after the provider revoked its scoped
				// Approval. The early check sees the revocation, nothing is applied
				// any more, and no drain restarts.
				expectOrphanedPublisherDelete()
				for i := range 2 {
					mockEarlyApprovalReads(l, map[string]earlyRead{"provider": {state: approvalv1.ApprovalStateRejected}})
					mockOwnedLists(nil, nil) // drainCapture inventory: nothing left
					start := len(fakeClient.Calls)
					l, err = reconcile(l)
					Expect(err).ToNot(HaveOccurred())
					Expect(l.Status.Draining).To(BeNil())
					Expect(l.Status.AppliedPlacement).To(BeNil())
					Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1 - i))
					Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
					Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
					expectNoCaptureCreates(start)
					ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
					Expect(ready).ToNot(BeNil())
					Expect(ready.Message).To(Equal("Approval has been revoked (provider gate, early restriction)"))
				}
			})

			It("drains capture the stale-child check keeps through the migration checkpoint before retirement", func() {
				f := provisionForRejection()
				l := f.listener
				l.Status.AuthorizationPolicyVersion = ""
				legacy := makeLegacyApproval(l, approvalv1.ApprovalStateGranted)
				legacyReq := makeLegacyRequest(l)
				liveRLs := []gatewayv1.RouteListener{f.rl}

				// R1: the legacy Approval makes the Listener not fresh and is recorded.
				// The children carry the current fingerprint, so the stale-child check
				// keeps them; both scoped gates grant and the migration checkpoints
				// their drain without deleting anything.
				mockEarlyRestrictionGranted(l)
				mockLegacyApprovalGets(legacy, 3)   // isFreshInstall, record, migration discovery
				mockLegacyRequestGets(legacyReq, 2) // record, migration discovery
				mockResolveTopology()
				mockOwnedLists(liveRLs, f.subs) // stale-child check: current fingerprint, kept
				mockApprovalGranted()
				mockOwnedLists(liveRLs, f.subs) // migration drain inventory
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(l.Status.AuthorizationMigration.Phase).To(Equal(handler.ExportMigrationPhaseDraining))
				Expect(l.Status.AuthorizationMigration.DrainStarted).To(BeTrue())
				d := l.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(d.Reason).To(Equal("legacy migration"))
				Expect(d.OldFingerprint).To(Equal(f.fingerprint))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&f.rl)}))
				Expect(d.OldSubscribers).To(ConsistOf(*ctypes.ObjectRefFromObject(&f.subs[0]), *ctypes.ObjectRefFromObject(&f.subs[1])))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)

				// R2-R5: continueDrain deletes the RouteListener, then the Subscribers,
				// with UID+RV preconditions.
				l = driveDrainToCleaningPublisher(l, f.rl, f.subs)

				// R6: the drain completes. The fingerprint and stale-child checks are
				// skipped while the migration drains (no owner-label List is stubbed);
				// the migration consumes the drain and only checkpoints the legacy
				// request's deletion.
				expectOrphanedPublisherDelete()
				mockEarlyRestrictionGranted(l)
				mockLegacyApprovalGets(legacy, 1)
				mockLegacyRequestGets(legacyReq, 2)
				mockResolveTopology()
				mockApprovalGranted()
				start = len(fakeClient.Calls)
				l, err = reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(l.Status.Draining).To(BeNil())
				Expect(l.Status.AppliedPlacement.Fingerprint).To(BeEmpty())
				Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1))
				Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
				Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
				expectNoCaptureCreates(start)
				expectRequestRetirementPrepared(l, legacyReq, start)
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
			// In these tests, the only capture candidate is rejected for its route mode
			// (pass-through/failover) and nothing was applied, so no candidate remains.
			// Children are only removed through the persisted drain; with nothing to
			// drain no Publisher is touched.
			It("should block with pass-through route and NOT create ApprovalRequest or children", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				// Delivery (A's zone) resolves before the per-candidate route mode check.
				mockGetObserverApp(makeConsumerApp())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockPassThroughRoute()

				mockOwnedLists(nil, nil) // drainCapture inventory: nothing to drain

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pass-through"))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
				Expect(listener.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countCalls(start, "CreateOrUpdate", nil)).To(BeZero())
				Expect(countNamespacedSubscriberLists(start)).To(BeZero())
			})

			It("should block with failover route and NOT create ApprovalRequest or children", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				// Delivery (A's zone) resolves before the per-candidate route mode check.
				mockGetObserverApp(makeConsumerApp())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockFailoverRoute()

				mockOwnedLists(nil, nil) // drainCapture inventory: nothing to drain

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failover"))
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
				Expect(listener.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countCalls(start, "CreateOrUpdate", nil)).To(BeZero())
				Expect(countNamespacedSubscriberLists(start)).To(BeZero())
			})

			It("should drain existing children through the checkpoint when Route transitions to pass-through", func() {
				// Partial provisioning: the refs are recorded but nothing was applied.
				genericPub := ctypes.ObjectRef{Name: util.MakePublisherName(util.GenericEventType), Namespace: listenerZoneStatus}
				rl := gatewayv1.RouteListener{ObjectMeta: metav1.ObjectMeta{
					Name: "old-rl", Namespace: listenerZoneStatus, UID: "rl-uid-9", ResourceVersion: "12",
				}}
				subs := []pubsubv1.Subscriber{{
					ObjectMeta: metav1.ObjectMeta{Name: "old-sub-rq", Namespace: listenerZoneStatus, UID: "sub-uid-9", ResourceVersion: "4"},
					Spec:       pubsubv1.SubscriberSpec{Publisher: genericPub},
				}}
				listener := newListener()
				listener.Status.RouteListener = &ctypes.ObjectRef{Name: "old-rl", Namespace: listenerZoneStatus}
				listener.Status.EventSubscriptions = []ctypes.ObjectRef{
					{Name: "old-sub-rq", Namespace: listenerZoneStatus},
				}

				// mockPassThroughTopology stubs the Once() reads of one reconcile that
				// rejects the only capture candidate as pass-through.
				mockPassThroughTopology := func() {
					mockGetConsumerApp(makeConsumerApp())
					mockGetProviderApp(makeProviderApp())
					mockGetSpectreApp(makeSpectreAppPtr())
					// Delivery (A's zone) resolves before the per-candidate route mode
					// check rejects the only candidate as pass-through.
					mockGetObserverApp(makeConsumerApp())
					mockGetEventStore(makeListenerEventStore())
					mockPassThroughRoute()
				}
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})

				// R1: the checkpoint is recorded; nothing is deleted yet.
				mockPassThroughTopology()
				mockOwnedLists([]gatewayv1.RouteListener{rl}, subs) // drainCapture inventory
				start := len(fakeClient.Calls)
				l, err := reconcile(listener)
				Expect(err).ToNot(HaveOccurred())
				d := l.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(d.Reason).To(Equal("no supported capture placement"))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&rl)}))
				Expect(d.OldSubscribers).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&subs[0])}))
				Expect(d.SourcePublisher).To(Equal(&genericPub))
				Expect(l.Status.RouteListener).ToNot(BeNil())
				Expect(l.Status.EventSubscriptions).To(HaveLen(1))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
				blocked := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeProcessing)
				Expect(blocked).ToNot(BeNil())
				Expect(blocked.Reason).To(Equal(condition.ReasonBlocked))
				Expect(blocked.Message).To(ContainSubstring("pass-through"))

				l = driveDrainToCleaningPublisher(l, rl, subs)

				// R6: the Publisher is cleaned in the Subscribers' namespace, the drain
				// completes and the reconcile falls through to the same rejection.
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.SubscriberList"), mock.Anything).
					Return(nil).Once()
				fakeClient.EXPECT().
					Delete(ctx, mock.AnythingOfType("*v1.Publisher"), mock.Anything).
					Run(func(_ context.Context, obj client.Object, _ ...client.DeleteOption) {
						Expect(obj.GetName()).To(Equal(genericPub.Name))
						Expect(obj.GetNamespace()).To(Equal(genericPub.Namespace))
					}).
					Return(nil).Once()
				mockPassThroughTopology()
				mockOwnedLists(nil, nil) // drainCapture inventory: nothing left
				start = len(fakeClient.Calls)
				l, err = reconcile(l)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pass-through"))
				Expect(l.Status.Draining).To(BeNil())
				Expect(l.Status.RouteListener).To(BeNil())
				Expect(l.Status.EventSubscriptions).To(BeEmpty())
				Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1))
				Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
				Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
				expectNoCaptureCreates(start)
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
			It("should checkpoint the drain before deleting anything and set AccessDenied", func() {
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

				// Denial checkpoint: one owner-label inventory, no deletes yet.
				mockOwnedLists(
					[]gatewayv1.RouteListener{{ObjectMeta: metav1.ObjectMeta{Name: "old-rl", Namespace: listenerZoneStatus, UID: "rl-uid-1"}}},
					[]pubsubv1.Subscriber{{ObjectMeta: metav1.ObjectMeta{Name: "old-sub-rq", Namespace: listenerZoneStatus, UID: "sub-uid-1"}}},
				)

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// Nothing is deleted before the checkpoint is persisted.
				Expect(listener.Status.Draining).ToNot(BeNil())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(listener.Status.Draining.Reason).To(Equal("approval denied"))
				Expect(listener.Status.Draining.OldRouteListeners).To(HaveLen(1))
				Expect(listener.Status.Draining.OldSubscribers).To(HaveLen(1))
				Expect(listener.Status.RouteListener).ToNot(BeNil())
				Expect(listener.Status.EventSubscriptions).To(HaveLen(1))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
			})
		})

		Context("dual-gate: provider Granted + consumer Denied", func() {
			It("should set AccessDenied and touch no capture child or Publisher when nothing exists", func() {
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

				// Denial: the owner-label inventory finds nothing to drain.
				mockOwnedLists(nil, nil)

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(listener.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countNamespacedSubscriberLists(start)).To(BeZero())
				expectNoCaptureCreates(start)
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
					mockOwnedLists(nil, nil) // identity drain inventory: nothing to stop

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

		// Finding #17: a scoped Approval that loses or changes its controller owner
		// after provisioning stays Granted but is never repaired by the approval
		// controller, so the applied capture it no longer authorizes is drained.
		Context("scoped identity: controller owner lost after provisioning", func() {
			foreignOwner := func() []metav1.OwnerReference {
				isController := true
				return []metav1.OwnerReference{{
					APIVersion: "v1", Kind: "ConfigMap", Name: "foreign-object", UID: "foreign-uid-000", Controller: &isController,
				}}
			}

			// mockGateReadError stubs one gate whose Approval read fails transiently.
			mockGateReadError := func() {
				fakeClient.EXPECT().
					CreateOrUpdate(ctx, mock.AnythingOfType("*v1.ApprovalRequest"), mock.Anything).
					Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
						_ = mutate()
					}).
					Return(controllerutil.OperationResultNone, nil).Once()
				fakeClient.EXPECT().
					List(ctx, mock.AnythingOfType("*v1.ApprovalRequestList"), mock.Anything).
					Return(nil).Once()
				fakeClient.EXPECT().
					Get(ctx, mock.AnythingOfType("types.NamespacedName"), mock.AnythingOfType("*v1.Approval")).
					Return(fmt.Errorf("internal API error")).Once()
			}

			DescribeTable("checkpoints the drain, deletes the RouteListener then the Subscribers, and stays drained",
				func(owners map[string][]metav1.OwnerReference, gates string) {
					f := provisionForRejection()
					liveRLs := []gatewayv1.RouteListener{f.rl}
					mockGates := func() {
						for _, key := range []string{"provider", "consumer"} {
							if refs, ok := owners[key]; ok {
								mockApprovalGrantedGateOwnedBy(key, refs)
							} else {
								mockApprovalGrantedGate(key)
							}
						}
					}
					expectIdentityErr := func(err error) {
						Expect(err).To(MatchError(builder.ErrScopedIdentity))
						Expect(err).To(MatchError(ContainSubstring("missing or foreign controller owner")))
						Expect(err.Error()).To(ContainSubstring("approval evaluation failed"))
					}

					// R1: the Approvals are still Granted, so the early check passes;
					// the build rejects the owner and only the checkpoint is written.
					mockEarlyRestrictionGranted(f.listener)
					mockResolveTopology()
					mockOwnedLists(liveRLs, f.subs) // stale-child check: same fingerprint, kept
					mockGates()
					mockOwnedLists(liveRLs, f.subs) // drainCapture inventory
					start := len(fakeClient.Calls)
					l, err := reconcile(f.listener)
					expectIdentityErr(err)
					d := l.Status.Draining
					Expect(d).ToNot(BeNil())
					Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
					Expect(d.Reason).To(Equal("approval identity invalid (" + gates + " gate)"))
					Expect(d.OldFingerprint).To(Equal(f.fingerprint))
					Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&f.rl)}))
					Expect(d.OldSubscribers).To(ConsistOf(*ctypes.ObjectRefFromObject(&f.subs[0]), *ctypes.ObjectRefFromObject(&f.subs[1])))
					Expect(l.Status.RouteListener).ToNot(BeNil())
					Expect(countCalls(start, "Delete", nil)).To(BeZero())
					expectNoCaptureCreates(start)

					// R2-R5: continueDrain deletes the RouteListener, then the Subscribers.
					l = driveDrainToCleaningPublisher(l, f.rl, f.subs)

					// R6 completes the drain and cleans the Publisher; R6 and R7 still
					// fail on the identity, find nothing to drain and create nothing.
					expectOrphanedPublisherDelete()
					for i := range 2 {
						mockEarlyRestrictionGranted(l)
						mockResolveTopology()
						mockOwnedLists(nil, nil) // stale-child check
						mockGates()
						mockOwnedLists(nil, nil) // drainCapture inventory: nothing left
						start = len(fakeClient.Calls)
						l, err = reconcile(l)
						expectIdentityErr(err)
						Expect(l.Status.Draining).To(BeNil())
						Expect(l.Status.AppliedPlacement).To(BeNil())
						Expect(l.Status.RouteListener).To(BeNil())
						Expect(l.Status.EventSubscriptions).To(BeEmpty())
						Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1 - i))
						Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
						Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
						expectNoCaptureCreates(start)
					}
				},
				Entry("provider Approval without ownerReferences", map[string][]metav1.OwnerReference{"provider": nil}, "provider"),
				Entry("consumer Approval without ownerReferences", map[string][]metav1.OwnerReference{"consumer": nil}, "consumer"),
				Entry("both Approvals without ownerReferences",
					map[string][]metav1.OwnerReference{"provider": nil, "consumer": nil}, "provider and consumer"),
				Entry("provider Approval with a foreign controller", map[string][]metav1.OwnerReference{"provider": foreignOwner()}, "provider"),
			)

			It("drains on the consumer identity failure when the provider gate fails transiently", func() {
				f := provisionForRejection()
				liveRLs := []gatewayv1.RouteListener{f.rl}
				mockEarlyRestrictionGranted(f.listener)
				mockResolveTopology()
				mockOwnedLists(liveRLs, f.subs) // stale-child check
				mockGateReadError()             // provider gate
				mockApprovalGrantedGateOwnedBy("consumer", nil)
				mockOwnedLists(liveRLs, f.subs) // drainCapture inventory
				start := len(fakeClient.Calls)
				l, err := reconcile(f.listener)
				// The consumer gate's identity failure stays matchable behind the
				// provider gate's error in the combined error.
				Expect(err).To(MatchError(builder.ErrScopedIdentity))
				Expect(err).To(MatchError(ContainSubstring("internal API error")))
				Expect(l.Status.Draining).ToNot(BeNil())
				Expect(l.Status.Draining.Reason).To(Equal("approval identity invalid (consumer gate)"))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
			})

			It("keeps applied capture on a transient gate error", func() {
				f := provisionForRejection()
				mockEarlyRestrictionGranted(f.listener)
				mockResolveTopology()
				mockOwnedLists([]gatewayv1.RouteListener{f.rl}, f.subs) // stale-child check
				mockGateReadError()                                     // provider gate
				mockApprovalGrantedGate("consumer")
				start := len(fakeClient.Calls)
				l, err := reconcile(f.listener)
				Expect(err).To(MatchError(ContainSubstring("internal API error")))
				Expect(err).ToNot(MatchError(builder.ErrScopedIdentity))
				Expect(err.Error()).To(ContainSubstring("approval evaluation failed"))
				Expect(l.Status.Draining).To(BeNil())
				Expect(l.Status.RouteListener).ToNot(BeNil())
				Expect(l.Status.AppliedPlacement.Fingerprint).To(Equal(f.fingerprint))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
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

				// The early read of the provider request fails, so only the
				// dual-gate build reports the rejection.
				mockEarlyApprovalsGranted(f.listener)
				mockEarlyRequestReads(f.listener, map[string]earlyRead{"provider": {err: errors.NewServiceUnavailable("etcd leader changed")}})
				mockResolveTopology()
				mockOwnedLists(liveRLs, f.subs) // stale-child check
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
				mockOwnedLists(nil, nil) // stale-child check
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
				mockOwnedLists(nil, nil) // stale-child check
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

		// Findings #2/#7: a rejected current ApprovalRequest stops applied capture
		// at the early check, before any topology read, so a NotReady Application
		// or Zone or a non-definitive placement error cannot keep it running.
		Context("early restriction: rejected current request while capture is applied", func() {
			// provisionWithRequestUIDs provisions a Listener (R0) and gives its
			// request refs the UIDs ensureApprovals records from the live requests.
			provisionWithRequestUIDs := func() *rejectionFixture {
				f := provisionForRejection()
				f.listener.Status.ProviderApprovalRequest.UID = "provider-req-uid"
				f.listener.Status.ConsumerApprovalRequest.UID = "consumer-req-uid"
				return f
			}

			// mockConsumerNotReady stubs the consumer Application read with C NotReady.
			mockConsumerNotReady := func() {
				app := makeConsumerApp()
				meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
					Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "NotReady",
				})
				mockGetConsumerApp(app)
			}

			// mockExposureListFails stubs the topology up to placement with the
			// provider binding's ApiExposure List failing with err.
			mockExposureListFails := func(err error) func() {
				return func() {
					mockGetConsumerApp(makeConsumerApp())
					mockGetProviderApp(makeProviderApp())
					mockGetSpectreApp(makeSpectreAppPtr())
					mockGetObserverApp(makeConsumerApp()) // A==C: observer resolves to consumer
					mockGetEventStore(makeListenerEventStore())
					mockGetCaptureRoute()
					fakeClient.EXPECT().
						List(ctx, mock.AnythingOfType("*v1.ApiExposureList"), mock.Anything).
						Return(err).Once()
				}
			}

			// drainOnEarlyRejection provisions (R0), rejects gate's current request
			// and runs R1-R7 with the topology broken by mockBroken from R6 on:
			// R1 checkpoints the drain from the early reads alone, R2-R5 delete the
			// RouteListener and then the Subscribers, R6 cleans the Publisher and
			// completes the drain, and R6-R7 return brokenErr without another drain.
			drainOnEarlyRejection := func(gate string, mockBroken func(), brokenErr string) *rejectionFixture {
				f := provisionWithRequestUIDs()
				liveRLs := []gatewayv1.RouteListener{f.rl}

				// R1: no topology read, no approval evaluation, nothing deleted.
				mockEarlyApprovalsGranted(f.listener)
				mockEarlyRequestReads(f.listener, map[string]earlyRead{gate: {state: approvalv1.ApprovalStateRejected}})
				mockOwnedLists(liveRLs, f.subs) // drainCapture inventory
				start := len(fakeClient.Calls)
				l, err := reconcile(f.listener)
				Expect(err).ToNot(HaveOccurred())
				d := l.Status.Draining
				Expect(d).ToNot(BeNil())
				Expect(d.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(d.Reason).To(Equal("approval request rejected (" + gate + " gate, early restriction)"))
				Expect(d.OldFingerprint).To(Equal(f.fingerprint))
				Expect(d.OldRouteListeners).To(Equal([]ctypes.ObjectRef{*ctypes.ObjectRefFromObject(&f.rl)}))
				Expect(d.OldSubscribers).To(ConsistOf(*ctypes.ObjectRefFromObject(&f.subs[0]), *ctypes.ObjectRefFromObject(&f.subs[1])))
				Expect(l.Status.RouteListener).ToNot(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countCalls(start, "CreateOrUpdate", nil)).To(BeZero())
				Expect(countCalls(start, "Get", &applicationv1.Application{})).To(BeZero())
				ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready).ToNot(BeNil())
				Expect(ready.Status).To(Equal(metav1.ConditionFalse))
				Expect(ready.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(ready.Message).To(Equal("ApprovalRequest has been denied (" + gate + " gate, early restriction)"))

				// R2-R5: RouteListener first (UID+RV preconditions), then Subscribers.
				l = driveDrainToCleaningPublisher(l, f.rl, f.subs)

				// R6 completes the drain and cleans the Publisher; R6 and R7 then hit
				// the broken topology. Nothing is applied any more, so no request is
				// read early and no drain restarts.
				expectOrphanedPublisherDelete()
				for i := range 2 {
					mockEarlyApprovalsGranted(l)
					mockBroken()
					start = len(fakeClient.Calls)
					l, err = reconcile(l)
					Expect(err).To(MatchError(ContainSubstring(brokenErr)))
					Expect(l.Status.Draining).To(BeNil())
					Expect(l.Status.AppliedPlacement.Fingerprint).To(BeEmpty())
					Expect(l.Status.RouteListener).To(BeNil())
					Expect(l.Status.EventSubscriptions).To(BeEmpty())
					Expect(countCalls(start, "Delete", &pubsubv1.Publisher{})).To(Equal(1 - i))
					Expect(countCalls(start, "Delete", &gatewayv1.RouteListener{})).To(BeZero())
					Expect(countCalls(start, "Delete", &pubsubv1.Subscriber{})).To(BeZero())
					Expect(countCalls(start, "Get", &approvalv1.ApprovalRequest{})).To(BeZero())
					Expect(countCalls(start, "CreateOrUpdate", nil)).To(BeZero())
				}
				f.listener = l
				return f
			}

			DescribeTable("drains before reading topology, stays drained while it is broken, and reaches ensureApprovals once it recovers",
				func(gate string, mockBroken func(), brokenErr string) {
					f := drainOnEarlyRejection(gate, mockBroken, brokenErr)

					// R8: topology recovers. ensureApprovals reports RequestDenied, the
					// drain inventory is empty and nothing is provisioned.
					mockEarlyApprovalsGranted(f.listener)
					mockResolveTopology()
					mockOwnedLists(nil, nil) // stale-child check
					mockRejectedGates(gate)
					mockOwnedLists(nil, nil) // drainCapture inventory: nothing left
					start := len(fakeClient.Calls)
					l, err := reconcile(f.listener)
					Expect(err).ToNot(HaveOccurred())
					Expect(countCalls(start, "CreateOrUpdate", &approvalv1.ApprovalRequest{})).To(Equal(2))
					Expect(l.Status.Draining).To(BeNil())
					Expect(l.Status.AppliedPlacement).To(BeNil())
					Expect(countCalls(start, "Delete", nil)).To(BeZero())
					expectNoCaptureCreates(start)
					expectRequestDenied(l, gate)
				},
				Entry("consumer request, C NotReady", "consumer", mockConsumerNotReady, "failed to resolve consumer Application"),
				Entry("provider request, C NotReady", "provider", mockConsumerNotReady, "failed to resolve consumer Application"),
				Entry("consumer request, ApiExposure List Forbidden", "consumer",
					mockExposureListFails(errors.NewForbidden(schema.GroupResource{Group: apiv1.GroupVersion.Group, Resource: "apiexposures"}, "", fmt.Errorf("RBAC"))),
					"failed to resolve placement"),
				Entry("provider request, ApiExposure List ServiceUnavailable", "provider",
					mockExposureListFails(errors.NewServiceUnavailable("etcd leader changed")),
					"failed to resolve placement"),
			)

			It("creates the new-intent ApprovalRequests after that drain (no deadlock)", func() {
				f := drainOnEarlyRejection("consumer", mockConsumerNotReady, "failed to resolve consumer Application")
				l := f.listener
				l.Spec.ApiListener.RequestFilter = &spectrev1.ListenerFilter{Trigger: map[string]string{"method": "POST"}}

				// The changed intent yields new request names; the Approvals are still
				// bound to the old requests, so both gates are Pending.
				var capP, capC approvalv1.ApprovalRequest
				mockEarlyApprovalsGranted(l)
				mockResolveTopology()
				mockOwnedLists(nil, nil) // stale-child check
				mockGateBoundToOldRequest("provider", f.providerReq, &capP)
				mockGateBoundToOldRequest("consumer", f.consumerReq, &capC)
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(countCalls(start, "CreateOrUpdate", &approvalv1.ApprovalRequest{})).To(Equal(2))
				Expect(capP.Name).To(HavePrefix("ar-v1-"))
				Expect(capP.Name).ToNot(Equal(f.providerReq.Name))
				Expect(capC.Name).To(HavePrefix("ar-v1-"))
				Expect(capC.Name).ToNot(Equal(f.consumerReq.Name))
				Expect(l.Status.ProviderApprovalRequest.Name).To(Equal(capP.Name))
				Expect(l.Status.ConsumerApprovalRequest.Name).To(Equal(capC.Name))
				ready := meta.FindStatusCondition(l.Status.Conditions, condition.ConditionTypeReady)
				Expect(ready).ToNot(BeNil())
				Expect(ready.Reason).To(Equal(condition.ReasonApprovalPending))
				Expect(l.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
			})

			DescribeTable("does not drain on a request that is not the recorded one",
				func(gate string, read earlyRead) {
					f := provisionWithRequestUIDs()
					mockEarlyApprovalsGranted(f.listener)
					mockEarlyRequestReads(f.listener, map[string]earlyRead{gate: read})
					mockConsumerNotReady()
					start := len(fakeClient.Calls)
					l, err := reconcile(f.listener)
					Expect(err).To(MatchError(ContainSubstring("failed to resolve consumer Application")))
					Expect(countCalls(start, "Get", &approvalv1.ApprovalRequest{})).To(Equal(2))
					Expect(l.Status.Draining).To(BeNil())
					Expect(l.Status.RouteListener).ToNot(BeNil())
					Expect(countCalls(start, "List", nil)).To(BeZero()) // no drain inventory
					Expect(countCalls(start, "Delete", nil)).To(BeZero())
					Expect(countCalls(start, "CreateOrUpdate", nil)).To(BeZero())
				},
				Entry("consumer request recreated with another UID", "consumer",
					earlyRead{state: approvalv1.ApprovalStateRejected, uid: "recreated-uid"}),
				Entry("provider request recreated with another UID", "provider",
					earlyRead{state: approvalv1.ApprovalStateRejected, uid: "recreated-uid"}),
				Entry("consumer request NotFound", "consumer",
					earlyRead{err: errors.NewNotFound(schema.GroupResource{Group: approvalv1.GroupVersion.Group, Resource: "approvalrequests"}, "")}),
				Entry("provider request NotFound", "provider",
					earlyRead{err: errors.NewNotFound(schema.GroupResource{Group: approvalv1.GroupVersion.Group, Resource: "approvalrequests"}, "")}),
			)

			It("reads no request and still runs ensureApprovals when nothing is applied", func() {
				l := newListener()
				l.Status.ProviderApproval = &ctypes.ObjectRef{Name: "ag-v1-provider", Namespace: listenerNamespace}
				l.Status.ConsumerApproval = &ctypes.ObjectRef{Name: "ag-v1-consumer", Namespace: listenerNamespace}
				l.Status.ProviderApprovalRequest = &ctypes.ObjectRef{Name: "ar-v1-provider", Namespace: listenerNamespace, UID: "provider-req-uid"}
				l.Status.ConsumerApprovalRequest = &ctypes.ObjectRef{Name: "ar-v1-consumer", Namespace: listenerNamespace, UID: "consumer-req-uid"}

				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockEarlyApprovalsGranted(l)
				mockResolveTopology()
				mockNoStaleChildren()
				mockRejectedGates("consumer")
				mockOwnedLists(nil, nil) // drainCapture inventory: nothing to drain
				start := len(fakeClient.Calls)
				l, err := reconcile(l)
				Expect(err).ToNot(HaveOccurred())
				Expect(countCalls(start, "Get", &approvalv1.ApprovalRequest{})).To(BeZero())
				Expect(countCalls(start, "CreateOrUpdate", &approvalv1.ApprovalRequest{})).To(Equal(2))
				Expect(l.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
				expectRequestDenied(l, "consumer")
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
			It("should still checkpoint the drain and return the combined error", func() {
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

				// Denial checkpoint: one owner-label inventory, no deletes yet.
				mockOwnedLists([]gatewayv1.RouteListener{{ObjectMeta: metav1.ObjectMeta{Name: "old-rl", Namespace: listenerZoneStatus, UID: "rl-uid-1"}}}, nil)

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				// The drain checkpoint is recorded, and the combined error from the
				// consumer gate is preserved and returned.
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("consumer gate"))

				// The drain still started; nothing is deleted before it is persisted.
				Expect(listener.Status.Draining).ToNot(BeNil())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(listener.Status.Draining.OldRouteListeners).To(HaveLen(1))
				Expect(listener.Status.RouteListener).ToNot(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)

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

				// Denial: the owner-label inventory finds nothing to drain.
				mockOwnedLists(nil, nil)

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// No provisioning happened.
				Expect(listener.Status.RouteListener).To(BeNil())
				Expect(listener.Status.EventSubscriptions).To(BeEmpty())
				Expect(listener.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)
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
			// capture and reports Ready. children are returned by the stale-child check.
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
			It("should drain stale children through the checkpoint before evaluating approval", func() {
				l := newListener()
				s := withStaleCapture(l, "old-fp", "")
				expectStaleDrainedBeforeApproval(l, s)
			})
		})

		Context("regression: unsupported route mode cleanup happens before approval", func() {
			It("should block without ever creating ApprovalRequests or touching a Publisher", func() {
				listener := newListener()
				mockGetConsumerApp(makeConsumerApp())
				mockGetProviderApp(makeProviderApp())
				mockGetSpectreApp(makeSpectreAppPtr())
				// Delivery (A's zone) resolves before the per-candidate route mode check.
				mockGetObserverApp(makeConsumerApp())
				mockGetZone()
				mockListEventConfigs([]eventv1.EventConfig{makeListenerEventConfig()})
				mockGetEventStore(makeListenerEventStore())
				mockPassThroughRoute()

				mockOwnedLists(nil, nil) // drainCapture inventory: nothing to drain

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				// Route mode rejection happens BEFORE approval.
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("pass-through"))

				// No approval-related status refs set.
				Expect(listener.Status.ProviderApproval).To(BeNil())
				Expect(listener.Status.ConsumerApproval).To(BeNil())
				Expect(listener.Status.ProviderApprovalRequest).To(BeNil())
				Expect(listener.Status.ConsumerApprovalRequest).To(BeNil())
				Expect(countCalls(start, "CreateOrUpdate", nil)).To(BeZero())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countNamespacedSubscriberLists(start)).To(BeZero())
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
			It("should checkpoint the drain without resolving Applications", func() {
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

				// handleDenialCleanup: one owner-label inventory, checkpoint only.
				mockOwnedLists(
					[]gatewayv1.RouteListener{{ObjectMeta: metav1.ObjectMeta{Name: "old-rl", Namespace: listenerZoneStatus, UID: "rl-uid-1"}}},
					[]pubsubv1.Subscriber{{ObjectMeta: metav1.ObjectMeta{Name: "old-sub-rq", Namespace: listenerZoneStatus, UID: "sub-uid-1"}}},
				)

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				// Drain recorded; the refs stay until Stopping deletes the children.
				Expect(listener.Status.Draining).ToNot(BeNil())
				Expect(listener.Status.Draining.Phase).To(Equal(handler.ExportDrainPhaseStopping))
				Expect(listener.Status.Draining.Reason).To(Equal("early restriction (provider gate)"))
				Expect(listener.Status.RouteListener).ToNot(BeNil())
				Expect(listener.Status.EventSubscriptions).To(HaveLen(1))
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				expectNoCaptureCreates(start)

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

				// handleDenialCleanup: the owner-label inventory finds nothing to
				// drain, so no Publisher is touched.
				mockOwnedLists(nil, nil)

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(listener.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countNamespacedSubscriberLists(start)).To(BeZero())
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

				// handleDenialCleanup: the owner-label inventory finds nothing to
				// drain, so no Publisher is touched.
				mockOwnedLists(nil, nil)

				start := len(fakeClient.Calls)
				err := h.CreateOrUpdate(ctx, listener)
				Expect(err).ToNot(HaveOccurred())

				readyCond := meta.FindStatusCondition(listener.Status.Conditions, condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal(condition.ReasonAccessDenied))
				Expect(listener.Status.Draining).To(BeNil())
				Expect(countCalls(start, "Delete", nil)).To(BeZero())
				Expect(countNamespacedSubscriberLists(start)).To(BeZero())
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
