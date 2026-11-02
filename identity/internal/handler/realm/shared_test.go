// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package realm

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/internal/leader"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// leaderNamespace holds the older Realm of the shared group, which is elected leader.
const leaderNamespace = "leader-env"

var _ = Describe("shared Realm leadership", func() {
	var (
		mockK8s *fake.MockJanitorClient
		ctx     context.Context
	)

	BeforeEach(func() {
		mockK8s = fake.NewMockJanitorClient(GinkgoT())
		ctx = newTestContext(mockK8s)
	})

	It("mirrors the elected leader without requesting a Keycloak service", func() {
		idp := newValidIdP()
		idp.Name = "test-idp"
		idp.Namespace = "default"
		sharedIDP := idp.DeepCopy()
		sharedIDP.Name = "leader-idp"
		sharedIDP.Namespace = leaderNamespace

		follower := newValidRealm()
		follower.CreationTimestamp = metav1.NewTime(time.Unix(2, 0))
		leaderRealm := follower.DeepCopy()
		leaderRealm.Namespace = leaderNamespace
		leaderRealm.CreationTimestamp = metav1.NewTime(time.Unix(1, 0))
		leaderRealm.Spec.IdentityProvider = types.ObjectRefFromObject(sharedIDP)
		leaderRealm.Status.IssuerUrl = "https://issuer.example.com/realms/test-realm"
		leaderRealm.SetCondition(condition.NewReadyCondition("Ready", "Realm is ready"))

		overrideSecretsGet(func(context.Context, string) (string, error) {
			return "", context.Canceled
		})
		mockK8s.EXPECT().
			Get(mock.Anything, k8stypes.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, obj client.Object, _ ...client.GetOption) {
				*obj.(*identityv1.IdentityProvider) = *idp
			}).
			Return(nil)

		handler := NewHandlerRealm(unusedFactory)
		handler.PeerReader = realmPeerReader(idp, sharedIDP, follower, leaderRealm)

		Expect(handler.CreateOrUpdate(ctx, follower)).To(Succeed())
		Expect(follower.Status.Leader).To(Equal(types.ObjectRefFromObject(leaderRealm)))
		Expect(follower.Status.IssuerUrl).To(Equal(leaderRealm.Status.IssuerUrl))
		Expect(follower.Status.ManagedAttributes).To(BeNil())
		Expect(meta.IsStatusConditionTrue(follower.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("reports not ready while the elected leader is not ready", func() {
		idp := newValidIdP()
		idp.Name = "test-idp"
		idp.Namespace = "default"
		leaderIDP := idp.DeepCopy()
		leaderIDP.Name = "leader-idp"
		leaderIDP.Namespace = leaderNamespace

		follower := newValidRealm()
		follower.CreationTimestamp = metav1.NewTime(time.Unix(2, 0))
		leaderRealm := follower.DeepCopy()
		leaderRealm.Namespace = leaderNamespace
		leaderRealm.CreationTimestamp = metav1.NewTime(time.Unix(1, 0))
		leaderRealm.Spec.IdentityProvider = types.ObjectRefFromObject(leaderIDP)

		mockK8s.EXPECT().
			Get(mock.Anything, k8stypes.NamespacedName{Namespace: "default", Name: "test-idp"}, mock.AnythingOfType("*v1.IdentityProvider"), mock.Anything).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, obj client.Object, _ ...client.GetOption) {
				*obj.(*identityv1.IdentityProvider) = *idp
			}).
			Return(nil)

		handler := NewHandlerRealm(unusedFactory)
		handler.PeerReader = realmPeerReader(idp, leaderIDP, follower, leaderRealm)

		err := handler.CreateOrUpdate(ctx, follower)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("waiting for Realm leader"))
		Expect(follower.Status.Leader).To(Equal(types.ObjectRefFromObject(leaderRealm)))
		Expect(meta.IsStatusConditionFalse(follower.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("does not delete a Keycloak realm while another Realm still uses it", func() {
		idp := newValidIdP()
		idp.Name = "test-idp"
		idp.Namespace = "default"

		deleting := newValidRealm()
		deleting.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		deleting.Finalizers = []string{"identity.example.com/finalizer"}
		deleting.Status.AdminUrl = idp.Status.AdminUrl
		active := deleting.DeepCopy()
		active.Namespace = leaderNamespace
		active.DeletionTimestamp = nil

		handler := NewHandlerRealm(unusedFactory)
		handler.PeerReader = realmPeerReader(idp, nil, deleting, active)

		Expect(handler.Delete(ctx, deleting)).To(Succeed())
	})
})

func realmPeerReader(idp, sharedIDP *identityv1.IdentityProvider, realms ...*identityv1.Realm) client.Reader {
	scheme := runtime.NewScheme()
	Expect(identityv1.AddToScheme(scheme)).To(Succeed())

	objects := make([]client.Object, 0, len(realms)+2)
	objects = append(objects, idp)
	if sharedIDP != nil {
		objects = append(objects, sharedIDP)
	}
	for _, realm := range realms {
		objects = append(objects, realm)
	}

	return k8sfake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&identityv1.IdentityProvider{}, leader.IndexFieldSpecAdminURL, leader.AdminURLIndexValue).
		WithIndex(&identityv1.Realm{}, leader.IndexFieldSpecIdentityProvider, func(obj client.Object) []string {
			realm := obj.(*identityv1.Realm)
			if realm.Spec.IdentityProvider == nil {
				return nil
			}
			return []string{realm.Spec.IdentityProvider.String()}
		}).
		WithObjects(objects...).
		Build()
}
