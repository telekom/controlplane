// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

var _ = Describe("shared Client leadership", func() {
	var (
		mockK8s *fake.MockJanitorClient
		ctx     context.Context
	)

	BeforeEach(func() {
		mockK8s = fake.NewMockJanitorClient(GinkgoT())
		ctx = newTestContext(mockK8s)
	})

	It("mirrors the leader UID when the resolved secrets match", func() {
		realm := newValidRealm()
		leaderClient := sharedClient(realm, "leader-client", "leader-secret", time.Unix(1, 0))
		leaderClient.Status.ClientUid = "keycloak-client-uid"
		leaderClient.SetCondition(condition.NewReadyCondition("Ready", "Client is ready"))

		follower := sharedClient(realm, "member-client", "follower-secret", time.Unix(2, 0))
		follower.Status.RotatedClientSecret = "stale-secret"
		follower.Status.SecretExpiresAt = &metav1.Time{Time: time.Unix(3, 0)}
		follower.SetCondition(metav1.Condition{
			Type: identityv1.SecretRotationConditionType, Status: metav1.ConditionTrue, Reason: "Rotated",
		})

		overrideSecretsGet(func(_ context.Context, ref string) (string, error) {
			if ref == "resolved-password" {
				return "resolved-password", nil
			}
			switch ref {
			case "follower-secret", "leader-secret":
				return "same-secret", nil
			case "resolved-password":
				return "resolved-password", nil
			default:
				return "", context.Canceled
			}
		})
		mockRealmGet(mockK8s, realm)

		handler := NewHandlerClient(unusedFactory)
		handler.PeerReader = clientPeerReader(realm, leaderClient, follower)

		Expect(handler.CreateOrUpdate(ctx, follower)).To(Succeed())
		Expect(follower.Status.Leader).To(Equal(types.ObjectRefFromObject(leaderClient)))
		Expect(follower.Status.ClientUid).To(Equal("keycloak-client-uid"))
		Expect(follower.Status.RotatedClientSecret).To(BeEmpty())
		Expect(follower.Status.SecretExpiresAt).To(BeNil())
		Expect(meta.FindStatusCondition(follower.Status.Conditions, identityv1.SecretRotationConditionType)).To(BeNil())
		Expect(meta.IsStatusConditionTrue(follower.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("blocks a follower whose resolved secret differs from the leader", func() {
		realm := newValidRealm()
		leaderClient := sharedClient(realm, "leader-client", "leader-secret", time.Unix(1, 0))
		leaderClient.SetCondition(condition.NewReadyCondition("Ready", "Client is ready"))
		follower := sharedClient(realm, "member-client", "follower-secret", time.Unix(2, 0))

		overrideSecretsGet(func(_ context.Context, ref string) (string, error) {
			if ref == "resolved-password" {
				return "resolved-password", nil
			}
			if ref == "leader-secret" {
				return "leader-value", nil
			}
			return "follower-value", nil
		})
		mockRealmGet(mockK8s, realm)

		handler := NewHandlerClient(unusedFactory)
		handler.PeerReader = clientPeerReader(realm, leaderClient, follower)

		err := handler.CreateOrUpdate(ctx, follower)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("client secret differs from leader"))
		Expect(follower.Status.Leader).To(Equal(types.ObjectRefFromObject(leaderClient)))
		Expect(meta.IsStatusConditionFalse(follower.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
	})

	It("does not delete a Keycloak Client while another Client still uses it", func() {
		realm := newValidRealm()
		leaderClient := sharedClient(realm, "leader-client", "leader-secret", time.Unix(1, 0))
		deleting := sharedClient(realm, "member-client", "follower-secret", time.Unix(2, 0))
		deleting.DeletionTimestamp = &metav1.Time{Time: time.Now()}
		deleting.Finalizers = []string{"identity.example.com/finalizer"}

		handler := NewHandlerClient(unusedFactory)
		handler.PeerReader = clientPeerReader(realm, leaderClient, deleting)

		Expect(handler.Delete(ctx, deleting)).To(Succeed())
	})
})

// sharedClient returns a Client of the shared group created at the given time; the oldest one leads.
func sharedClient(realm *identityv1.Realm, name, secret string, created time.Time) *identityv1.Client {
	return &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         realm.Namespace,
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec: identityv1.ClientSpec{
			Realm:        types.ObjectRefFromObject(realm),
			ClientId:     "shared-client-id",
			ClientSecret: secret,
		},
	}
}

func clientPeerReader(realm *identityv1.Realm, clients ...*identityv1.Client) client.Reader {
	scheme := runtime.NewScheme()
	Expect(identityv1.AddToScheme(scheme)).To(Succeed())

	peerRealm := realm.DeepCopy()
	peerRealm.Status.AdminUrl = ""
	idp := &identityv1.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "test-idp", Namespace: realm.Namespace},
		Spec:       identityv1.IdentityProviderSpec{AdminUrl: realm.Status.AdminUrl},
	}
	objects := make([]client.Object, 0, len(clients)+2)
	objects = append(objects, peerRealm, idp)
	for _, identityClient := range clients {
		objects = append(objects, identityClient)
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
		WithIndex(&identityv1.Client{}, leader.IndexFieldSpecRealm, func(obj client.Object) []string {
			identityClient := obj.(*identityv1.Client)
			if identityClient.Spec.Realm == nil {
				return nil
			}
			return []string{identityClient.Spec.Realm.String()}
		}).
		WithObjects(objects...).
		Build()
}
