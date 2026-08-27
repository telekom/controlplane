// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/event/internal/handler/eventconfig"
	"github.com/telekom/controlplane/event/internal/handler/util"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The callback Route's ACL allows util.CallbackClientName, and the gateway's
// jwt-keycloak plugin runs with consumer_match enabled and
// consumer_match_ignore_not_found disabled. A token is therefore rejected
// unless a gateway Consumer of that name exists, so creating the identity
// Client alone leaves callback delivery permanently broken with a 403.
var _ = Describe("EventConfigHandler callback gateway Consumer", func() {
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

		fakeClient.EXPECT().Scheme().Return(buildScheme()).Maybe()

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

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Return(nil).Maybe()

		// Stop the run at the readiness gate; this spec only asserts the Consumer.
		fakeClient.EXPECT().AllReady().Return(false).Maybe()

		for _, kind := range []string{"*v1.Client", "*v1.EventStore", "*v1.Route"} {
			fakeClient.EXPECT().
				CreateOrUpdate(ctx, mock.AnythingOfType(kind), mock.Anything).
				Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
					_ = mutate()
				}).
				Return(controllerutil.OperationResultCreated, nil).Maybe()
		}
	})

	It("creates a gateway Consumer for the callback client", func() {
		var created *gatewayv1.Consumer
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Consumer"), mock.Anything).
			Run(func(_ context.Context, o client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				created = o.(*gatewayv1.Consumer)
			}).
			Return(controllerutil.OperationResultCreated, nil).Once()

		_ = h.CreateOrUpdate(ctx, obj)

		Expect(created).ToNot(BeNil(), "no gateway Consumer was created for the callback client")
		Expect(created.Spec.Name).To(Equal(util.CallbackClientName))
		Expect(created.Name).To(Equal(util.CallbackClientName))
		// Without this the Consumer cannot be pushed to any gateway.
		Expect(created.Spec.Gateway.Name).To(Equal("gw"))
	})
})
