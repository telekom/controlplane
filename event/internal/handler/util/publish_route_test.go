// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	mock "github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/config"
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
		eventConfig *eventv1.EventConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)

		eventConfig = &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ec-zone-a",
				Namespace: "zone-a-ns", // must match zone.Status.Namespace for SetControllerReference
				UID:       k8stypes.UID("ec-uid-1234"),
			},
			Spec: eventv1.EventConfigSpec{
				Local: &eventv1.LocalBackend{PublishEventUrl: "http://publish-service:8080/events"},
			},
		}
	})

	It("should return BlockedError when zone has no default preset", func() {
		zoneNoPreset := makeZoneNoPreset("zone-a", "zone-a-ns")

		route, err := util.CreatePublishRoute(ctx, zoneNoPreset, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no default preset"))
	})

	It("should return BlockedError when zone has no gateway reference in status", func() {
		zoneNoGw := makeZoneNoGateway("zone-a", "zone-a-ns")

		route, err := util.CreatePublishRoute(ctx, zoneNoGw, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		rootCause := unwrapAll(err)
		Expect(rootCause).To(Satisfy(isBlockedError))
		Expect(err.Error()).To(ContainSubstring("has no gateway reference in status"))
	})

	It("should return error when publishEventUrl is invalid", func() {
		zone := makeZone("zone-a", "zone-a-ns")

		badConfig := &eventv1.EventConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "ec-bad", Namespace: "default"},
			Spec: eventv1.EventConfigSpec{
				Local: &eventv1.LocalBackend{PublishEventUrl: "://bad-url"},
			},
		}

		route, err := util.CreatePublishRoute(ctx, zone, badConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to parse publishEventUrl"))
	})

	It("should create publish route successfully", func() {
		zone := makeZone("zone-a", "zone-a-ns")

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

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig, util.WithOwner(eventConfig))
		Expect(err).ToNot(HaveOccurred())
		Expect(route).ToNot(BeNil())

		// Verify route metadata
		Expect(route.Name).To(Equal("publish"))
		Expect(route.Namespace).To(Equal("zone-a-ns"))

		// Verify labels
		Expect(route.Labels).To(HaveKeyWithValue(config.DomainLabelKey, "event"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("zone"), "zone-a"))
		Expect(route.Labels).To(HaveKeyWithValue(config.BuildLabelKey("type"), "publish"))

		// Verify upstream: from publishEventUrl
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("http"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("publish-service"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(8080)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/events"))

		// Verify hostnames and paths from preset
		Expect(route.Spec.Hostnames).To(HaveLen(1))
		Expect(route.Spec.Hostnames[0]).To(Equal("gateway.example.com"))
		Expect(route.Spec.Paths).To(HaveLen(2))
		Expect(route.Spec.Paths[0]).To(Equal("/horizon/events/v1"))
		Expect(route.Spec.Paths[1]).To(Equal("/horizon/publish/v1"))

		// Verify Security
		Expect(route.Spec.Security.DisableAccessControl).To(BeTrue())

		// Verify GatewayRef
		Expect(route.Spec.GatewayRef.Name).To(Equal("gateway-zone-a"))
		Expect(route.Spec.GatewayRef.Namespace).To(Equal("default"))

		// Verify owner reference was set (via SetControllerReference)
		Expect(route.GetOwnerReferences()).To(HaveLen(1))
		Expect(route.GetOwnerReferences()[0].Name).To(Equal("ec-zone-a"))
		Expect(route.GetOwnerReferences()[0].UID).To(Equal(k8stypes.UID("ec-uid-1234")))
	})

	It("should create publish route with HTTPS upstream URL", func() {
		zone := makeZone("zone-a", "zone-a-ns")

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
				Local: &eventv1.LocalBackend{PublishEventUrl: "https://publish-service.internal:9443/api/publish"},
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
		Expect(route.Spec.Backend.Upstreams).To(HaveLen(1))
		Expect(route.Spec.Backend.Upstreams[0].Scheme).To(Equal("https"))
		Expect(route.Spec.Backend.Upstreams[0].Hostname).To(Equal("publish-service.internal"))
		Expect(route.Spec.Backend.Upstreams[0].Port).To(Equal(int32(9443)))
		Expect(route.Spec.Backend.Upstreams[0].Path).To(Equal("/api/publish"))
	})

	It("should return error when CreateOrUpdate fails", func() {
		zone := makeZone("zone-a", "zone-a-ns")

		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Return(controllerutil.OperationResultNone, fmt.Errorf("create failed"))

		route, err := util.CreatePublishRoute(ctx, zone, eventConfig)
		Expect(err).To(HaveOccurred())
		Expect(route).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("failed to create or update Route"))
		Expect(err.Error()).To(ContainSubstring("create failed"))
	})
})
