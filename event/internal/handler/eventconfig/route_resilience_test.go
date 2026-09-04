// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventconfig_test

import (
	"context"
	"fmt"

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
	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Routes are derived from the Zone and the EventConfig spec alone. Reconciling
// them in the same fail-fast chain as the identity Clients and the EventStore
// meant one unusable client name froze route rendering for the whole zone, with
// no way to recover other than editing the CR by hand.
var _ = Describe("EventConfigHandler route resilience", func() {
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

		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.EventConfigList")).
			Return(nil).Maybe()
	})

	It("still renders Routes when the identity Client cannot be created", func() {
		// The failure that wedged the spectre-test vCluster: an identity Client
		// name already owned by another domain.
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Client"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
			}).
			Return(controllerutil.OperationResultNone, fmt.Errorf(`clients.identity.cp.ei.telekom.de "rover" already exists`)).Maybe()

		routes := 0
		fakeClient.EXPECT().
			CreateOrUpdate(ctx, mock.AnythingOfType("*v1.Route"), mock.Anything).
			Run(func(_ context.Context, _ client.Object, mutate controllerutil.MutateFn) {
				_ = mutate()
				routes++
			}).
			Return(controllerutil.OperationResultCreated, nil).Maybe()

		err := h.CreateOrUpdate(ctx, obj)

		// The backend failure is still reported, so the object is requeued.
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("already exists"))
		// But route rendering is no longer collateral damage.
		Expect(routes).To(BeNumerically(">", 0), "no Route was rendered after the backend failure")
	})
})
