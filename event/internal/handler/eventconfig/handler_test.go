// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventconfig"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

func newEventConfig() *eventv1.EventConfig {
	return &eventv1.EventConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-eventconfig",
			Namespace: "default",
			UID:       "test-uid-1234",
		},
		Spec: eventv1.EventConfigSpec{
			Zone: ctypes.ObjectRef{
				Name:      "test-zone",
				Namespace: "default",
			},
			Admin: eventv1.AdminConfig{
				Url: "https://admin.example.com",
				Client: eventv1.ClientConfig{
					Realm: ctypes.ObjectRef{
						Name:      "test-realm",
						Namespace: "default",
					},
				},
			},
			ServerSendEventUrl: "https://sse.example.com",
			PublishEventUrl:    "http://publish.internal:8080/publish",
			VoyagerApiUrl:      "http://voyager.internal:8080/voyager",
			Mesh: eventv1.MeshConfig{
				FullMesh: false,
				Client: eventv1.ClientConfig{
					Realm: ctypes.ObjectRef{
						Name:      "test-realm",
						Namespace: "default",
					},
				},
			},
		},
	}
}

var (
	realmKey   = k8stypes.NamespacedName{Name: "test-realm", Namespace: "default"}
	zoneKey    = k8stypes.NamespacedName{Name: "test-zone", Namespace: "default"}
	gwRealmKey = k8stypes.NamespacedName{Name: "gw-realm", Namespace: "default"}
)

func makeReadyRealm() *identityv1.Realm {
	r := &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-realm",
			Namespace: "default",
		},
		Status: identityv1.RealmStatus{
			IssuerUrl: "https://issuer.example.com",
		},
	}
	return r
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

func makeReadyGatewayRealm() *gatewayv1.Realm {
	r := &gatewayv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-realm",
			Namespace: "default",
		},
		Spec: gatewayv1.RealmSpec{
			Url:       "https://gateway.example.com:443",
			IssuerUrl: "https://issuer.example.com",
		},
	}
	meta.SetStatusCondition(&r.Status.Conditions, metav1.Condition{
		Type:   condition.ConditionTypeReady,
		Status: metav1.ConditionTrue,
		Reason: "Ready",
	})
	return r
}

// buildScheme creates a runtime.Scheme with all types needed by the handler.
func buildScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = eventv1.AddToScheme(s)
	_ = gatewayv1.AddToScheme(s)
	return s
}

var _ = Describe("EventConfigHandler", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *eventconfig.EventConfigHandler
		obj        *eventv1.EventConfig
		testScheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &eventconfig.EventConfigHandler{}
		obj = newEventConfig()
		testScheme = buildScheme()
	})

	// mockGetRealm sets up a mock for c.Get on the identity realm key.
	mockGetRealm := func(realm *identityv1.Realm, times int) {
		fakeClient.EXPECT().
			Get(ctx, realmKey, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Realm) = *realm
			}).
			Return(nil).Times(times)
	}

	// mockGetRealmError sets up a mock for c.Get on the identity realm key that returns an error.
	mockGetRealmError := func(err error) {
		fakeClient.EXPECT().
			Get(ctx, realmKey, mock.AnythingOfType("*v1.Realm")).
			Return(err).Once()
	}

	// mockCreateOrUpdateClient sets up a mock for c.CreateOrUpdate on an identity Client.
	mockCreateOrUpdateClient := func(result controllerutil.OperationResult, err error, times int) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Client"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Times(times)
	}

	// mockCreateOrUpdateEventStore sets up a mock for c.CreateOrUpdate on an EventStore.
	mockCreateOrUpdateEventStore := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.EventStore"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	// mockGetZone sets up a mock for c.Get on the zone key (adminv1.Zone).
	mockGetZone := func(zone *adminv1.Zone, times int) {
		fakeClient.EXPECT().
			Get(ctx, zoneKey, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil).Times(times)
	}

	// mockGetZoneError sets up a mock for c.Get on the zone key that returns an error.
	mockGetZoneError := func(err error) {
		fakeClient.EXPECT().
			Get(ctx, zoneKey, mock.AnythingOfType("*v1.Zone")).
			Return(err).Once()
	}

	// mockListEventConfigs sets up a mock for c.List on EventConfigList.
	mockListEventConfigs := func(items []eventv1.EventConfig, times int) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{
					Items: items,
				}
			}).
			Return(nil).Times(times)
	}

	// mockListEventConfigsError sets up a mock for c.List that returns an error.
	mockListEventConfigsError := func(err error) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Return(err).Once()
	}

	// mockGetGatewayRealm sets up a mock for c.Get on the gateway realm key.
	mockGetGatewayRealm := func(realm *gatewayv1.Realm, times int) {
		fakeClient.EXPECT().
			Get(ctx, gwRealmKey, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayv1.Realm) = *realm
			}).
			Return(nil).Times(times)
	}

	// mockScheme sets up a mock for c.Scheme() used by SetControllerReference in route mutators.
	mockScheme := func() {
		fakeClient.EXPECT().Scheme().Return(testScheme).Maybe()
	}

	// mockCreateOrUpdateCallbackRoute sets up a mock for the callback Route CreateOrUpdate.
	// It populates Spec.Downstreams so the handler can read the URL.
	mockCreateOrUpdateCallbackRoute := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	// mockCreateOrUpdateVoyagerRoute sets up a mock for the voyager Route CreateOrUpdate.
	mockCreateOrUpdateVoyagerRoute := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	// mockCreateOrUpdatePublishRoute sets up a mock for the publish Route CreateOrUpdate.
	mockCreateOrUpdatePublishRoute := func(result controllerutil.OperationResult, err error) {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(result, err).Once()
	}

	// setupFullHappyPath sets up all mocks needed for a full successful CreateOrUpdate run
	// (up to the AllReady / CleanupAll point).
	setupFullHappyPath := func() {
		realm := makeReadyRealm()
		zone := makeReadyZone()
		gwRealm := makeReadyGatewayRealm()

		mockScheme()
		mockGetRealm(realm, 2) // admin + mesh
		mockCreateOrUpdateClient(controllerutil.OperationResultCreated, nil, 2)
		mockCreateOrUpdateEventStore(controllerutil.OperationResultCreated, nil)
		mockGetZone(zone, 3)                             // callback + voyager + publish
		mockListEventConfigs([]eventv1.EventConfig{}, 2) // callback + voyager
		mockGetGatewayRealm(gwRealm, 3)                  // callback + voyager + publish
		mockCreateOrUpdateCallbackRoute(controllerutil.OperationResultCreated, nil)
		mockCreateOrUpdateVoyagerRoute(controllerutil.OperationResultCreated, nil)
		mockCreateOrUpdatePublishRoute(controllerutil.OperationResultCreated, nil)
	}

	Describe("CreateOrUpdate", func() {

		It("should return BlockedError when Realm is not found", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Group: "identity.cp.ei.telekom.de", Resource: "realms"},
				"test-realm",
			)
			mockGetRealmError(notFoundErr)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced identity Realm"))
			Expect(isBlockedError(err)).To(BeTrue())
		})

		It("should return error when Realm Get fails", func() {
			mockGetRealmError(fmt.Errorf("connection refused"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get identity Realm"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("should return error when admin identity Client creation fails", func() {
			realm := makeReadyRealm()
			mockGetRealm(realm, 1)

			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Client"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("create failed")).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create identity Client"))
		})

		It("should return error when mesh identity Client creation fails", func() {
			realm := makeReadyRealm()
			mockGetRealm(realm, 2)

			// First call (admin client) succeeds
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Client"), mock.Anything).
				Return(controllerutil.OperationResultCreated, nil).Once()

			// Second call (mesh client) fails
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Client"), mock.Anything).
				Return(controllerutil.OperationResultNone, fmt.Errorf("mesh error")).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create identity Client"))
		})

		It("should return error when EventStore creation fails", func() {
			realm := makeReadyRealm()
			mockGetRealm(realm, 2)
			mockScheme()
			mockCreateOrUpdateClient(controllerutil.OperationResultCreated, nil, 2)
			mockCreateOrUpdateEventStore(controllerutil.OperationResultNone, fmt.Errorf("eventstore error"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create EventStore"))
		})

		It("should return error when GetZone fails in createCallbackRoutes", func() {
			realm := makeReadyRealm()
			mockGetRealm(realm, 2)
			mockScheme()
			mockCreateOrUpdateClient(controllerutil.OperationResultCreated, nil, 2)
			mockCreateOrUpdateEventStore(controllerutil.OperationResultCreated, nil)
			mockGetZoneError(fmt.Errorf("zone fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create callback Routes"))
		})

		It("should return BlockedError when zone namespace does not match", func() {
			realm := makeReadyRealm()
			zone := makeReadyZone()
			zone.Status.Namespace = "wrong-ns"

			mockGetRealm(realm, 2)
			mockScheme()
			mockCreateOrUpdateClient(controllerutil.OperationResultCreated, nil, 2)
			mockCreateOrUpdateEventStore(controllerutil.OperationResultCreated, nil)
			mockGetZone(zone, 1)

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must be located in the correlated zone-namespace"))
			Expect(isBlockedError(err)).To(BeTrue())
		})

		It("should return error when List EventConfigs fails in createCallbackRoutes", func() {
			realm := makeReadyRealm()
			zone := makeReadyZone()

			mockGetRealm(realm, 2)
			mockScheme()
			mockCreateOrUpdateClient(controllerutil.OperationResultCreated, nil, 2)
			mockCreateOrUpdateEventStore(controllerutil.OperationResultCreated, nil)
			mockGetZone(zone, 1)
			mockListEventConfigsError(fmt.Errorf("list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create callback Routes"))
		})

		It("should return error when createVoyagerRoutes GetZone fails", func() {
			realm := makeReadyRealm()
			zone := makeReadyZone()
			gwRealm := makeReadyGatewayRealm()

			mockGetRealm(realm, 2)
			mockScheme()
			mockCreateOrUpdateClient(controllerutil.OperationResultCreated, nil, 2)
			mockCreateOrUpdateEventStore(controllerutil.OperationResultCreated, nil)

			// Callback routes succeed: GetZone(1) + List(1) + GetGatewayRealm(1) + CreateOrUpdate Route(1)
			mockGetZone(zone, 1)
			mockListEventConfigs([]eventv1.EventConfig{}, 1)
			mockGetGatewayRealm(gwRealm, 1)
			mockCreateOrUpdateCallbackRoute(controllerutil.OperationResultCreated, nil)

			// Voyager routes: GetZone fails
			mockGetZoneError(fmt.Errorf("voyager zone fetch failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create voyager Routes"))
		})

		It("should return error when List EventConfigs fails in createVoyagerRoutes", func() {
			realm := makeReadyRealm()
			zone := makeReadyZone()
			gwRealm := makeReadyGatewayRealm()

			mockGetRealm(realm, 2)
			mockScheme()
			mockCreateOrUpdateClient(controllerutil.OperationResultCreated, nil, 2)
			mockCreateOrUpdateEventStore(controllerutil.OperationResultCreated, nil)

			// Callback routes succeed: GetZone(1) + List(1) + GetGatewayRealm(1) + CreateOrUpdate Route(1)
			mockGetZone(zone, 2) // callback + voyager
			mockListEventConfigs([]eventv1.EventConfig{}, 1)
			mockGetGatewayRealm(gwRealm, 1)
			mockCreateOrUpdateCallbackRoute(controllerutil.OperationResultCreated, nil)

			// Voyager routes: List fails
			mockListEventConfigsError(fmt.Errorf("voyager list failed"))

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create voyager Routes"))
		})

		It("should set NotReady condition when not all children are ready", func() {
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

		It("should return error when CleanupAll fails", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(true).Once()
			fakeClient.EXPECT().CleanupAll(ctx, mock.Anything).Return(0, fmt.Errorf("cleanup error")).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to cleanup old child resources"))
		})

		It("should set Ready condition when all children are ready and cleanup succeeds", func() {
			setupFullHappyPath()
			fakeClient.EXPECT().AllReady().Return(true).Once()
			fakeClient.EXPECT().CleanupAll(ctx, mock.Anything).Return(0, nil).Once()

			err := h.CreateOrUpdate(ctx, obj)

			Expect(err).ToNot(HaveOccurred())

			readyCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeReady)
			Expect(readyCond).ToNot(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("EventConfigProvisioned"))

			processingCond := meta.FindStatusCondition(obj.GetConditions(), condition.ConditionTypeProcessing)
			Expect(processingCond).ToNot(BeNil())
			Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(processingCond.Reason).To(Equal("Done"))
		})
	})

	Describe("Delete", func() {
		It("should always return nil", func() {
			err := h.Delete(ctx, obj)

			Expect(err).ToNot(HaveOccurred())
		})
	})
})
