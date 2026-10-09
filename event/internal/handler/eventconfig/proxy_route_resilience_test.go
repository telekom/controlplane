// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventconfig"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Proxy zones take a different branch through the reconcile: they get no admin
// identity Client, and their EventStore authenticates against the target zone.
// The split between the event backend and route rendering must hold there too.
var _ = Describe("EventConfigHandler proxy route resilience", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
		h          *eventconfig.EventConfigHandler
		obj        *eventv1.EventConfig
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctx = contextutil.WithEnv(ctx, "test-env")
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
		h = &eventconfig.EventConfigHandler{}

		obj = newEventConfig()
		// Turn it into a proxy EventConfig: Local and Proxy are mutually
		// exclusive per the CEL rule on the spec.
		obj.Spec.Local = nil
		obj.Spec.Proxy = &eventv1.ProxyBackend{
			TargetZone: ctypes.ObjectRef{Name: "target-zone", Namespace: "default"},
		}

		fakeClient.EXPECT().Scheme().Return(buildScheme()).Maybe()
		fakeClient.EXPECT().AllReady().Return(false).Maybe()

		zone := makeReadyZone()
		fakeClient.EXPECT().
			Get(ctx, zoneKey, mock.AnythingOfType("*v1.Zone")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*adminv1.Zone) = *zone
			}).
			Return(nil).Maybe()

		realm := makeReadyRealm()
		fakeClient.EXPECT().
			Get(ctx, realmKey, mock.AnythingOfType("*v1.Realm")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*identityv1.Realm) = *realm
			}).
			Return(nil).Maybe()

		// A proxy EventConfig resolves its target zone's EventConfig by listing.
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{}
			}).
			Return(nil).Maybe()
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				*list.(*eventv1.EventConfigList) = eventv1.EventConfigList{}
			}).
			Return(nil).Maybe()
		fakeClient.EXPECT().
			Get(ctx, mock.Anything, mock.AnythingOfType("*v1.EventConfig")).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				target := newEventConfig()
				target.ObjectMeta = metav1.ObjectMeta{Name: "target", Namespace: "default"}
				*out.(*eventv1.EventConfig) = *target
			}).
			Return(nil).Maybe()
	})

	It("still renders Routes on a proxy zone when the backend fails", func() {
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Client"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultNone, fmt.Errorf("identity client unavailable")).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Maybe()
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.EventStore"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultCreated, nil).Maybe()

		routes := 0
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				routes++
			}).
			Return(controllerutil.OperationResultCreated, nil).Maybe()

		err := h.CreateOrUpdate(ctx, obj)

		Expect(err).To(HaveOccurred())
		Expect(routes).To(BeNumerically(">", 0), "no Route was rendered on the proxy path after the backend failure")
		// Proxy zones must never get a local admin client.
		Expect(obj.Status.AdminClient).To(BeNil())
	})
})
