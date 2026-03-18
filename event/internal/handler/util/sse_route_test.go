// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mock "github.com/stretchr/testify/mock"
	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ---------- CreateSSERoute ----------

var _ = Describe("CreateSSERoute", func() {
	var (
		ctx         context.Context
		fakeClient  *fakeclient.MockJanitorClient
		zone        *adminv1.Zone
		eventConfig *eventv1.EventConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		zone = makeZone("zone-a", "default", "zone-a-ns", "gw-realm-a", "default")
		eventConfig = &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-a", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				ServerSendEventUrl: "http://sse-service:8080/sse",
			},
		}
	})

	It("should return BlockedError when zone has no GatewayRealm", func() {
		zoneNoRealm := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "zone-no-realm", Namespace: "default"},
			Status: adminv1.ZoneStatus{
				Namespace:    "zone-ns",
				GatewayRealm: nil,
			},
		}

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zoneNoRealm, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no GatewayRealm configured"))
	})

	It("should return BlockedError when realm is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "realms"}, "gw-realm-a"))

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return error when realm Get fails", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(fmt.Errorf("connection refused"))

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get realm"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return BlockedError when realm is not ready", func() {
		notReadyRealm := makeNotReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *notReadyRealm
			}).
			Return(nil)

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should return error when ServerSendEventUrl is invalid", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		badConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-bad", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				ServerSendEventUrl: "://bad-url",
			},
		}

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, badConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to parse ServerSendEventUrl"))
	})

	It("should create SSE route successfully", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		// makeSSERouteName("de.telekom.test.v1") = "sse--de-telekom-test-v1"
		Expect(route.Name).To(Equal("sse--de-telekom-test-v1"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("realm"), "gw-realm-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "sse"))
		Expect(route.Labels).To(HaveKey(eventv1.EventTypeLabelKey))

		// Verify upstream: from ServerSendEventUrl
		Expect(route.Spec.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Upstreams[0].Scheme).To(Equal("http"))
		Expect(route.Spec.Upstreams[0].Host).To(Equal("sse-service"))
		Expect(route.Spec.Upstreams[0].Port).To(Equal(8080))
		Expect(route.Spec.Upstreams[0].Path).To(Equal("/sse"))

		// Verify downstream: from realm URL
		Expect(route.Spec.Downstreams).To(HaveLen(1))
		Expect(route.Spec.Downstreams[0].Host).To(Equal("gateway.example.com"))
		Expect(route.Spec.Downstreams[0].Port).To(Equal(443))
		Expect(route.Spec.Downstreams[0].Path).To(Equal("/sse/v1/de.telekom.test.v1"))
		Expect(route.Spec.Downstreams[0].IssuerUrl).To(Equal("https://issuer.example.com"))

		// Verify Security
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())
		Expect(route.Spec.Security.DefaultConsumers).To(BeEmpty())

		// Verify Buffering: response buffering must be disabled for SSE streaming
		Expect(route.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
		Expect(route.Spec.Buffering.DisableRequestBuffering).To(BeFalse())

		// Verify realm ref
		Expect(route.Spec.Realm.Name).To(Equal("gw-realm-a"))
		Expect(route.Spec.Realm.Namespace).To(Equal("default"))
	})

	It("should add MeshClientName to DefaultConsumers when isTargetOfProxy is true", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DefaultConsumers).To(ContainElement(util.MeshClientName))
	})

	It("should NOT add MeshClientName to DefaultConsumers when isTargetOfProxy is false", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DefaultConsumers).To(BeEmpty())
	})

	It("should return error when CreateOrUpdate fails", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateSSERoute(ctx, "de.telekom.test.v1", zone, eventConfig, false)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update SSE Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CleanupOldSSERoutes ----------

var _ = Describe("CleanupOldSSERoutes", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return 0 deleted when Cleanup removes nothing", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(0, nil)

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(deleted).To(Equal(0))
	})

	It("should return count of deleted routes on success", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(3, nil)

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).ToNot(HaveOccurred())
		Expect(deleted).To(Equal(3))
	})

	It("should return error when Cleanup fails", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(0, fmt.Errorf("cleanup failed"))

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).To(HaveOccurred())
		Expect(deleted).To(Equal(0))
		Expect(err.Error()).To(ContainSubstring("failed to cleanup old SSE Routes"))
		Expect(err.Error()).To(ContainSubstring("de.telekom.test.v1"))
	})

	It("should return partial count when Cleanup fails after some deletions", func() {
		fakeClient.EXPECT().
			Cleanup(ctx, mock.AnythingOfType("*v1.RouteList"), mock.Anything).
			Return(2, fmt.Errorf("partial cleanup failure"))

		deleted, err := util.CleanupOldSSERoutes(ctx, "de.telekom.test.v1")
		Expect(err).To(HaveOccurred())
		Expect(deleted).To(Equal(2))
		Expect(err.Error()).To(ContainSubstring("failed to cleanup old SSE Routes"))
	})
})

// ---------- CreateSSEProxyRoute ----------

var _ = Describe("CreateSSEProxyRoute", func() {
	var (
		ctx            context.Context
		fakeClient     *fakeclient.MockJanitorClient
		subscriberZone *adminv1.Zone
		providerZone   *adminv1.Zone
		eventConfig    *eventv1.EventConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		subscriberZone = makeZone("zone-sub", "default", "zone-sub-ns", "gw-realm-sub", "default")
		providerZone = makeZone("zone-prov", "default", "zone-prov-ns", "gw-realm-prov", "default")
		eventConfig = &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-zone-prov", Namespace: "default"},
			Status: eventv1.EventConfigStatus{
				MeshClient: &eventv1.ObservedObjectRef{
					ObjectRef: ctypes.ObjectRef{Name: "mesh-client", Namespace: "zone-prov-ns"},
				},
			},
		}
	})

	It("should return BlockedError when subscriber zone has no GatewayRealm", func() {
		subscriberZoneNoRealm := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "zone-no-realm", Namespace: "default"},
			Status: adminv1.ZoneStatus{
				Namespace:    "zone-ns",
				GatewayRealm: nil,
			},
		}

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZoneNoRealm, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("subscriber zone"))
		Expect(err.Error()).To(ContainSubstring("has no GatewayRealm configured"))
	})

	It("should return BlockedError when subscriber realm is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "realms"}, "gw-realm-sub"))

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("subscriber realm"))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return error when subscriber realm Get fails", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(fmt.Errorf("connection refused"))

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get subscriber realm"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return BlockedError when subscriber realm is not ready", func() {
		notReadyRealm := makeNotReadyGatewayRealm("gw-realm-sub", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *notReadyRealm
			}).
			Return(nil)

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("subscriber realm"))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should return BlockedError when provider zone has no GatewayRealm", func() {
		readySubRealm := makeReadyGatewayRealm("gw-realm-sub", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySubRealm
			}).
			Return(nil)

		providerZoneNoRealm := &adminv1.Zone{
			ObjectMeta: metav1.ObjectMeta{Name: "zone-prov-no-realm", Namespace: "default"},
			Status: adminv1.ZoneStatus{
				Namespace:    "zone-prov-ns",
				GatewayRealm: nil,
			},
		}

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZoneNoRealm)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("provider zone"))
		Expect(err.Error()).To(ContainSubstring("has no GatewayRealm configured"))
	})

	It("should return BlockedError when provider realm is not found", func() {
		readySubRealm := makeReadyGatewayRealm("gw-realm-sub", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySubRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-prov", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "realms"}, "gw-realm-prov"))

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("provider realm"))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return BlockedError when provider realm is not ready", func() {
		readySubRealm := makeReadyGatewayRealm("gw-realm-sub", "default")
		notReadyProvRealm := makeNotReadyGatewayRealm("gw-realm-prov", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySubRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-prov", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *notReadyProvRealm
			}).
			Return(nil)

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("provider realm"))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should return error when mesh client Get fails", func() {
		readySubRealm := makeReadyGatewayRealm("gw-realm-sub", "default")
		readyProvRealm := makeReadyGatewayRealm("gw-realm-prov", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySubRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-prov", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyProvRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "mesh-client", Namespace: "zone-prov-ns"}, mock.AnythingOfType("*v1.Client")).
			Return(fmt.Errorf("client not found"))

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get gateway identity client"))
		Expect(err.Error()).To(ContainSubstring("client not found"))
	})

	It("should create SSE proxy route successfully", func() {
		readySubRealm := makeReadyGatewayRealm("gw-realm-sub", "default")
		readyProvRealm := makeReadyGatewayRealm("gw-realm-prov", "default")

		meshClient := &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{Name: "mesh-client", Namespace: "zone-prov-ns"},
			Spec: identityv1.ClientSpec{
				ClientId:     "mesh-id",
				ClientSecret: "mesh-secret",
			},
			Status: identityv1.ClientStatus{
				IssuerUrl: "https://issuer.provider.example.com",
			},
		}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySubRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-prov", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyProvRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "mesh-client", Namespace: "zone-prov-ns"}, mock.AnythingOfType("*v1.Client")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Client) = *meshClient
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata: created in subscriber zone's namespace
		Expect(route.Name).To(Equal("sse--de-telekom-test-v1"))
		Expect(route.Namespace).To(Equal("zone-sub-ns"))

		// Verify labels reference subscriber zone
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-sub"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("realm"), "gw-realm-sub"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "sse-proxy"))
		Expect(route.Labels).To(HaveKey(eventv1.EventTypeLabelKey))

		// Verify upstream: from provider realm with mesh client credentials
		Expect(route.Spec.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Upstreams[0].Scheme).To(Equal("https"))
		Expect(route.Spec.Upstreams[0].Host).To(Equal("gateway.example.com"))
		Expect(route.Spec.Upstreams[0].Port).To(Equal(443))
		Expect(route.Spec.Upstreams[0].Path).To(Equal("/sse/v1/de.telekom.test.v1"))
		Expect(route.Spec.Upstreams[0].ClientId).To(Equal("mesh-id"))
		Expect(route.Spec.Upstreams[0].ClientSecret).To(Equal("mesh-secret"))
		Expect(route.Spec.Upstreams[0].IssuerUrl).To(Equal("https://issuer.provider.example.com"))

		// Verify downstream: from subscriber realm
		Expect(route.Spec.Downstreams).To(HaveLen(1))
		Expect(route.Spec.Downstreams[0].Host).To(Equal("gateway.example.com"))
		Expect(route.Spec.Downstreams[0].Port).To(Equal(443))
		Expect(route.Spec.Downstreams[0].Path).To(Equal("/sse/v1/de.telekom.test.v1"))
		Expect(route.Spec.Downstreams[0].IssuerUrl).To(Equal("https://issuer.example.com"))

		// Verify Security
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())

		// Verify Buffering: response buffering must be disabled for SSE streaming
		Expect(route.Spec.Buffering.DisableResponseBuffering).To(BeTrue())
		Expect(route.Spec.Buffering.DisableRequestBuffering).To(BeFalse())

		// Verify realm ref points to subscriber realm
		Expect(route.Spec.Realm.Name).To(Equal("gw-realm-sub"))
		Expect(route.Spec.Realm.Namespace).To(Equal("default"))
	})

	It("should return error when CreateOrUpdate fails", func() {
		readySubRealm := makeReadyGatewayRealm("gw-realm-sub", "default")
		readyProvRealm := makeReadyGatewayRealm("gw-realm-prov", "default")

		meshClient := &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{Name: "mesh-client", Namespace: "zone-prov-ns"},
			Spec: identityv1.ClientSpec{
				ClientId:     "mesh-id",
				ClientSecret: "mesh-secret",
			},
			Status: identityv1.ClientStatus{
				IssuerUrl: "https://issuer.provider.example.com",
			},
		}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-sub", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySubRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-prov", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyProvRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "mesh-client", Namespace: "zone-prov-ns"}, mock.AnythingOfType("*v1.Client")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Client) = *meshClient
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateSSEProxyRoute(ctx, "de.telekom.test.v1", eventConfig, subscriberZone, providerZone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update SSE proxy Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})
