// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	mock "github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ---------- CreatePublishRoute ----------

var _ = Describe("CreatePublishRoute", func() {
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

		zone = makeZone("zone-a", "zone-a-ns", "gw-realm-a")
		eventConfig = &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ec-zone-a",
				Namespace: "zone-a-ns", // must match zone.Status.Namespace for SetControllerReference
				UID:       k8stypes.UID("ec-uid-1234"),
			},
			Spec: eventv1.EventConfigSpec{
				PublishEventUrl: "http://publish-service:8080/events",
			},
		}
	})

	It("should return BlockedError when realm is not found", func() {
		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "realms"}, "gw-realm-a"))

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig)
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

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to get realm"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return BlockedError when realm is not ready", func() {
		notReadyRealm := makeNotReadyGatewayRealm("gw-realm-a")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *notReadyRealm
			}).
			Return(nil)

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("not ready"))
	})

	It("should return error when publishEventUrl is invalid", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		badConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-bad", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				PublishEventUrl: "://bad-url",
			},
		}

		route, err := util.CreatePublishRoute(ctx, zone, badConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to parse publishEventUrl"))
	})

	It("should create publish route successfully", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		// SetControllerReference requires a scheme that knows both EventConfig and Route
		s := runtime.NewScheme()
		Expect(eventv1.AddToScheme(s)).To(Succeed())
		Expect(gatewayapi.AddToScheme(s)).To(Succeed())
		fakeClient.EXPECT().Scheme().Return(s).Maybe()

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("publish"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("realm"), "gw-realm-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "publish"))

		// Verify upstream: from publishEventUrl
		Expect(route.Spec.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Upstreams[0].Scheme).To(Equal("http"))
		Expect(route.Spec.Upstreams[0].Host).To(Equal("publish-service"))
		Expect(route.Spec.Upstreams[0].Port).To(Equal(8080))
		Expect(route.Spec.Upstreams[0].Path).To(Equal("/events"))

		// Verify downstream: from realm URL
		Expect(route.Spec.Downstreams).To(HaveLen(1))
		Expect(route.Spec.Downstreams[0].Host).To(Equal("gateway.example.com"))
		Expect(route.Spec.Downstreams[0].Port).To(Equal(443))
		Expect(route.Spec.Downstreams[0].Path).To(Equal("/zone-a/publish/v1"))
		Expect(route.Spec.Downstreams[0].IssuerUrl).To(Equal("https://issuer.example.com"))

		// Verify Security
		Expect(route.Spec.Security).ToNot(BeNil())
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())

		// Verify realm ref
		Expect(route.Spec.Realm.Name).To(Equal("gw-realm-a"))
		Expect(route.Spec.Realm.Namespace).To(Equal("default"))

		// Verify owner reference was set (via SetControllerReference)
		Expect(route.GetOwnerReferences()).To(HaveLen(1))
		Expect(route.GetOwnerReferences()[0].Name).To(Equal("ec-zone-a"))
		Expect(route.GetOwnerReferences()[0].UID).To(Equal(k8stypes.UID("ec-uid-1234")))
	})

	It("should create publish route with HTTPS upstream URL", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		s := runtime.NewScheme()
		Expect(eventv1.AddToScheme(s)).To(Succeed())
		Expect(gatewayapi.AddToScheme(s)).To(Succeed())
		fakeClient.EXPECT().Scheme().Return(s).Maybe()

		httpsConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ec-zone-a",
				Namespace: "zone-a-ns", // must match zone.Status.Namespace for SetControllerReference
				UID:       k8stypes.UID("ec-uid-1234"),
			},
			Spec: eventv1.EventConfigSpec{
				PublishEventUrl: "https://publish-service.internal:9443/api/publish",
			},
		}

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreatePublishRoute(ctx, zone, httpsConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify upstream with explicit HTTPS port
		Expect(route.Spec.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Upstreams[0].Scheme).To(Equal("https"))
		Expect(route.Spec.Upstreams[0].Host).To(Equal("publish-service.internal"))
		Expect(route.Spec.Upstreams[0].Port).To(Equal(9443))
		Expect(route.Spec.Upstreams[0].Path).To(Equal("/api/publish"))
	})

	It("should return error when CreateOrUpdate fails", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update publish Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})

	It("should use correct ObjectRef for realm in route spec", func() {
		readyRealm := makeReadyGatewayRealm("gw-realm-a")

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "gw-realm-a", Namespace: "default"}, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Realm) = *readyRealm
			}).
			Return(nil)

		s := runtime.NewScheme()
		Expect(eventv1.AddToScheme(s)).To(Succeed())
		Expect(gatewayapi.AddToScheme(s)).To(Succeed())
		fakeClient.EXPECT().Scheme().Return(s).Maybe()

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			RunAndReturn(func(_ context.Context, obj client.Object, mutate controllerutil.MutateFn) (controllerutil.OperationResult, error) {
				err := mutate()
				return controllerutil.OperationResultCreated, err
			})

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig)
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify realm ObjectRef
		expectedRealmRef := *ctypes.ObjectRefFromObject(readyRealm)
		Expect(route.Spec.Realm).To(Equal(expectedRealmRef))
	})
})
