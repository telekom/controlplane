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
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func makeReadyGatewayRealm(name, ns string) *gatewayapi.Realm {
	r := &gatewayapi.Realm{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: gatewayapi.RealmSpec{
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

func makeNotReadyGatewayRealm(name, ns string) *gatewayapi.Realm {
	return &gatewayapi.Realm{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: gatewayapi.RealmSpec{
			Url:       "https://gateway.example.com:443",
			IssuerUrl: "https://issuer.example.com",
		},
	}
}

func makeZone(name, ns, statusNs string, gwRealmName, gwRealmNs string) *adminv1.Zone {
	return &adminv1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status: adminv1.ZoneStatus{
			Namespace:    statusNs,
			GatewayRealm: &ctypes.ObjectRef{Name: gwRealmName, Namespace: gwRealmNs},
		},
	}
}

// ---------- CreateCallbackRoute ----------

var _ = Describe("CreateCallbackRoute", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		zone       *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		zone = makeZone("zone-a", "default", "zone-a-ns", "gw-realm-a", "default")
	})

	It("should return BlockedError when realm is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "realms"}, "gw-realm-a"))

		route, err := util.CreateCallbackRoute(ctx, zone)
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

		route, err := util.CreateCallbackRoute(ctx, zone)
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

		route, err := util.CreateCallbackRoute(ctx, zone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should create callback route successfully", func() {
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

		route, err := util.CreateCallbackRoute(ctx, zone)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("callback--zone-a"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("realm"), "gw-realm-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "callback"))

		// Verify upstream: localhost:8080/proxy
		Expect(route.Spec.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Upstreams[0].Scheme).To(Equal("http"))
		Expect(route.Spec.Upstreams[0].Host).To(Equal("localhost"))
		Expect(route.Spec.Upstreams[0].Port).To(Equal(8080))
		Expect(route.Spec.Upstreams[0].Path).To(Equal("/proxy"))

		// Verify downstream: from realm
		Expect(route.Spec.Downstreams).To(HaveLen(1))
		Expect(route.Spec.Downstreams[0].Host).To(Equal("gateway.example.com"))
		Expect(route.Spec.Downstreams[0].Port).To(Equal(443))
		Expect(route.Spec.Downstreams[0].Path).To(Equal("/zone-a/callback/v1"))
		Expect(route.Spec.Downstreams[0].IssuerUrl).To(Equal("https://issuer.example.com"))

		// Verify DynamicUpstream
		Expect(route.Spec.Traffic.DynamicUpstream).ToNot(BeNil())
		Expect(route.Spec.Traffic.DynamicUpstream.QueryParameter).To(Equal("callback"))

		// Verify Security
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())
		Expect(route.Spec.Security.DefaultConsumers).To(BeEmpty())

		// Verify realm ref
		Expect(route.Spec.Realm.Name).To(Equal("gw-realm-a"))
		Expect(route.Spec.Realm.Namespace).To(Equal("default"))
	})

	It("should add util.MeshClientName to DefaultConsumers when IsProxyTarget", func() {
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

		route, err := util.CreateCallbackRoute(ctx, zone, util.WithProxyTarget(true))
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DefaultConsumers).To(ContainElement("eventstore"))
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

		route, err := util.CreateCallbackRoute(ctx, zone)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update callback Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CreateProxyCallbackRoute ----------

var _ = Describe("CreateProxyCallbackRoute", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		sourceZone *adminv1.Zone
		targetZone *adminv1.Zone
		meshClient *identityv1.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		sourceZone = makeZone("zone-a", "default", "zone-a-ns", "gw-realm-a", "default")
		targetZone = makeZone("zone-b", "default", "zone-b-ns", "gw-realm-b", "default")
		meshClient = &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{Name: util.MeshClientName, Namespace: "zone-b-ns"},
			Spec: identityv1.ClientSpec{
				ClientId:     "mesh-client-id",
				ClientSecret: "mesh-client-secret",
			},
			Status: identityv1.ClientStatus{
				IssuerUrl: "https://issuer.target.example.com",
			},
		}
	})

	It("should return BlockedError when downstream realm is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "realms"}, "gw-realm-a"))

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return error when downstream realm Get fails", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(fmt.Errorf("connection refused"))

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get realm"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return BlockedError when downstream realm is not ready", func() {
		notReadyRealm := makeNotReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *notReadyRealm
			}).
			Return(nil)

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should return BlockedError when upstream realm is not found", func() {
		readySourceRealm := makeReadyGatewayRealm("gw-realm-a", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySourceRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-b", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "realms"}, "gw-realm-b"))

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should return BlockedError when upstream realm is not ready", func() {
		readySourceRealm := makeReadyGatewayRealm("gw-realm-a", "default")
		notReadyTargetRealm := makeNotReadyGatewayRealm("gw-realm-b", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySourceRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-b", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *notReadyTargetRealm
			}).
			Return(nil)

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should create proxy callback route successfully", func() {
		readySourceRealm := makeReadyGatewayRealm("gw-realm-a", "default")
		readyTargetRealm := makeReadyGatewayRealm("gw-realm-b", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySourceRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-b", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyTargetRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("callback--zone-b"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels reference source zone
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("realm"), "gw-realm-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "callback-proxy"))

		// Verify upstream: from target realm with mesh client credentials
		Expect(route.Spec.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Upstreams[0].Scheme).To(Equal("https"))
		Expect(route.Spec.Upstreams[0].Host).To(Equal("gateway.example.com"))
		Expect(route.Spec.Upstreams[0].Port).To(Equal(443))
		Expect(route.Spec.Upstreams[0].Path).To(Equal("/zone-b/callback/v1"))
		Expect(route.Spec.Upstreams[0].ClientId).To(Equal("mesh-client-id"))
		Expect(route.Spec.Upstreams[0].ClientSecret).To(Equal("mesh-client-secret"))
		Expect(route.Spec.Upstreams[0].IssuerUrl).To(Equal("https://issuer.target.example.com"))

		// Verify downstream: from source realm
		Expect(route.Spec.Downstreams).To(HaveLen(1))
		Expect(route.Spec.Downstreams[0].Host).To(Equal("gateway.example.com"))
		Expect(route.Spec.Downstreams[0].Port).To(Equal(443))
		Expect(route.Spec.Downstreams[0].Path).To(Equal("/zone-b/callback/v1"))
		Expect(route.Spec.Downstreams[0].IssuerUrl).To(Equal("https://issuer.example.com"))

		// Verify Security
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())

		// Verify realm ref points to downstream (source) realm
		Expect(route.Spec.Realm.Name).To(Equal("gw-realm-a"))
		Expect(route.Spec.Realm.Namespace).To(Equal("default"))
	})

	It("should return error when CreateOrUpdate fails", func() {
		readySourceRealm := makeReadyGatewayRealm("gw-realm-a", "default")
		readyTargetRealm := makeReadyGatewayRealm("gw-realm-b", "default")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySourceRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-b", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyTargetRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreateProxyCallbackRoute(ctx, sourceZone, targetZone, meshClient)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update proxy callback Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})

// ---------- CreateCallbackProxyRoutes ----------

var _ = Describe("CreateCallbackProxyRoutes", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		sourceZone *adminv1.Zone
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		sourceZone = makeZone("zone-a", "default", "zone-a-ns", "gw-realm-a", "default")
	})

	It("should return empty map when no target zones after filtering", func() {
		meshConfig := eventv1.MeshConfig{
			FullMesh:  false,
			ZoneNames: []string{},
		}
		targetZones := []*adminv1.Zone{
			makeZone("zone-b", "default", "zone-b-ns", "gw-realm-b", "default"),
		}

		routes, err := util.CreateCallbackProxyRoutes(ctx, meshConfig, sourceZone, targetZones)
		Expect(err).ToNot(HaveOccurred())
		Expect(routes).To(BeEmpty())
	})

	It("should skip source zone in full mesh", func() {
		meshConfig := eventv1.MeshConfig{FullMesh: true}
		targetZoneB := makeZone("zone-b", "default", "zone-b-ns", "gw-realm-b", "default")
		// Include source zone in targets to test skipping
		targetZones := []*adminv1.Zone{sourceZone, targetZoneB}

		meshClientObj := &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{Name: util.MeshClientName, Namespace: "zone-b-ns"},
			Spec: identityv1.ClientSpec{
				ClientId:     "mesh-client-id",
				ClientSecret: "mesh-client-secret",
			},
			Status: identityv1.ClientStatus{
				IssuerUrl: "https://issuer.target.example.com",
			},
		}

		// Get mesh client for zone-b
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: util.MeshClientName, Namespace: "zone-b-ns"}, mock.AnythingOfType("*v1.Client")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Client) = *meshClientObj
			}).
			Return(nil)

		// Get source realm (downstream) for proxy route creation
		readySourceRealm := makeReadyGatewayRealm("gw-realm-a", "default")
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySourceRealm
			}).
			Return(nil)

		// Get target realm (upstream)
		readyTargetRealm := makeReadyGatewayRealm("gw-realm-b", "default")
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-b", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyTargetRealm
			}).
			Return(nil)

		// CreateOrUpdate for proxy route
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		routes, err := util.CreateCallbackProxyRoutes(ctx, meshConfig, sourceZone, targetZones)
		Expect(err).ToNot(HaveOccurred())
		Expect(routes).To(HaveLen(1))
		Expect(routes).To(HaveKey("zone-b"))
	})

	It("should return error when mesh client Get fails", func() {
		meshConfig := eventv1.MeshConfig{FullMesh: true}
		targetZoneB := makeZone("zone-b", "default", "zone-b-ns", "gw-realm-b", "default")
		targetZones := []*adminv1.Zone{targetZoneB}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: util.MeshClientName, Namespace: "zone-b-ns"}, mock.AnythingOfType("*v1.Client")).
			Return(fmt.Errorf("client not found"))

		routes, err := util.CreateCallbackProxyRoutes(ctx, meshConfig, sourceZone, targetZones)
		Expect(err).To(HaveOccurred())
		Expect(routes).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get mesh client credentials"))
		Expect(err.Error()).To(ContainSubstring("client not found"))
	})

	It("should create routes for multiple target zones", func() {
		meshConfig := eventv1.MeshConfig{FullMesh: true}
		targetZoneB := makeZone("zone-b", "default", "zone-b-ns", "gw-realm-b", "default")
		targetZoneC := makeZone("zone-c", "default", "zone-c-ns", "gw-realm-c", "default")
		targetZones := []*adminv1.Zone{targetZoneB, targetZoneC}

		meshClientB := &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{Name: util.MeshClientName, Namespace: "zone-b-ns"},
			Spec: identityv1.ClientSpec{
				ClientId:     "mesh-client-b-id",
				ClientSecret: "mesh-client-b-secret",
			},
			Status: identityv1.ClientStatus{
				IssuerUrl: "https://issuer.b.example.com",
			},
		}
		meshClientC := &identityv1.Client{
			ObjectMeta: metav1.ObjectMeta{Name: util.MeshClientName, Namespace: "zone-c-ns"},
			Spec: identityv1.ClientSpec{
				ClientId:     "mesh-client-c-id",
				ClientSecret: "mesh-client-c-secret",
			},
			Status: identityv1.ClientStatus{
				IssuerUrl: "https://issuer.c.example.com",
			},
		}

		// Get mesh client for zone-b
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: util.MeshClientName, Namespace: "zone-b-ns"}, mock.AnythingOfType("*v1.Client")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Client) = *meshClientB
			}).
			Return(nil).Once()

		// Get source realm for zone-b proxy route
		readySourceRealm := makeReadyGatewayRealm("gw-realm-a", "default")
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readySourceRealm
			}).
			Return(nil).Times(2) // called for both zone-b and zone-c

		// Get target realm for zone-b proxy route
		readyRealmB := makeReadyGatewayRealm("gw-realm-b", "default")
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-b", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealmB
			}).
			Return(nil).Once()

		// CreateOrUpdate for zone-b proxy route
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		// Get mesh client for zone-c
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: util.MeshClientName, Namespace: "zone-c-ns"}, mock.AnythingOfType("*v1.Client")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Client) = *meshClientC
			}).
			Return(nil).Once()

		// Get target realm for zone-c proxy route
		readyRealmC := makeReadyGatewayRealm("gw-realm-c", "default")
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-c", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealmC
			}).
			Return(nil).Once()

		// CreateOrUpdate for zone-c proxy route
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		routes, err := util.CreateCallbackProxyRoutes(ctx, meshConfig, sourceZone, targetZones)
		Expect(err).ToNot(HaveOccurred())
		Expect(routes).To(HaveLen(2))
		Expect(routes).To(HaveKey("zone-b"))
		Expect(routes).To(HaveKey("zone-c"))
	})
})
