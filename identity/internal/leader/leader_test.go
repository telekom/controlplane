// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package leader

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLeader(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Identity Leader Suite")
}

var _ = Describe("leader election", func() {
	It("normalizes equivalent admin URLs", func() {
		normalized, err := NormalizeAdminURL(" HTTPS://KEYCLOAK.EXAMPLE.COM:443/auth/admin/realms/ ")

		Expect(err).NotTo(HaveOccurred())
		Expect(normalized).To(Equal("https://keycloak.example.com/auth/admin/realms"))
	})

	It("groups realms by normalized admin URL and realm name", func() {
		idpA := identityProvider("env-a", "idp", "https://KEYCLOAK.example.com:443/auth/")
		idpB := identityProvider("env-b", "idp", "https://keycloak.example.com/auth")
		idpOther := identityProvider("env-c", "idp", "https://other.example.com/auth")
		realmA := realm("env-a--zone-a", idpA, time.Unix(1, 0))
		realmB := realm("env-b--zone-b", idpB, time.Unix(2, 0))
		realmOther := realm("env-c--zone-c", idpOther, time.Unix(0, 0))
		reader := indexedReader(idpA, idpB, idpOther, realmA, realmB, realmOther)

		peers, err := RealmPeers(context.Background(), reader, realmA, idpA)

		Expect(err).NotTo(HaveOccurred())
		Expect(peers).To(HaveLen(2))
		Expect(ElectRealm(peers)).To(HaveField("Name", "rover"))
		Expect(ElectRealm(peers).Namespace).To(Equal(realmA.Namespace))
	})

	It("elects the oldest candidate without reordering candidates", func() {
		older := identityClient("env-b--zone", "service", nil, time.Unix(1, 0))
		newer := identityClient("env-a--zone", "service", nil, time.Unix(100, 0))
		candidates := []identityv1.Client{*newer, *older}

		elected := ElectClient(candidates)

		Expect(elected).To(HaveField("Namespace", "env-b--zone"))
		Expect(candidates[0].Namespace).To(Equal("env-a--zone"))
		Expect(ElectClient(nil)).To(BeNil())
	})

	It("breaks creation timestamp ties by namespace, then name", func() {
		created := time.Unix(10, 0)
		otherNamespace := identityClient("env-b--zone", "another", nil, created)
		laterName := identityClient("env-a--zone", "service", nil, created)
		earlierName := identityClient("env-a--zone", "another", nil, created)

		elected := ElectClient([]identityv1.Client{*otherNamespace, *laterName, *earlierName})

		Expect(elected).To(HaveField("Namespace", "env-a--zone"))
		Expect(elected).To(HaveField("Name", "another"))
	})

	It("ignores peers that are being deleted", func() {
		idpA := identityProvider("env-a", "idp", "https://keycloak.example.com/auth")
		idpB := identityProvider("env-b", "idp", "https://keycloak.example.com/auth")
		deleting := realm("env-a--zone", idpA, time.Unix(0, 0))
		deleting.DeletionTimestamp = &metav1.Time{Time: time.Unix(5, 0)}
		deleting.Finalizers = []string{"test/finalizer"}
		live := realm("env-b--zone", idpB, time.Unix(1, 0))
		reader := indexedReader(idpA, idpB, deleting, live)

		peers, err := RealmPeers(context.Background(), reader, live, idpB)

		Expect(err).NotTo(HaveOccurred())
		Expect(peers).To(HaveLen(1))
		Expect(ElectRealm(peers)).To(HaveField("Namespace", "env-b--zone"))
	})

	It("includes peers of the last reconciled admin URL", func() {
		idpOld := identityProvider("env-a", "idp-old", "https://old.example.com/auth")
		idpNew := identityProvider("env-b", "idp", "https://new.example.com/auth")
		oldPeer := realm("env-a--zone", idpOld, time.Unix(0, 0))
		moved := realm("env-b--zone", idpNew, time.Unix(1, 0))
		moved.Status.AdminUrl = "https://OLD.example.com:443/auth/"
		reader := indexedReader(idpOld, idpNew, oldPeer, moved)

		peers, err := RealmPeersForRealm(context.Background(), reader, moved)

		Expect(err).NotTo(HaveOccurred())
		Expect(peers).To(HaveLen(2))
		Expect(ElectRealm(peers)).To(HaveField("Namespace", "env-a--zone"))
	})

	It("selects the same client across realms", func() {
		realmA := identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "rover", Namespace: "env-a--zone-a"}}
		realmB := identityv1.Realm{ObjectMeta: metav1.ObjectMeta{Name: "rover", Namespace: "env-b--zone-b"}}
		created := time.Unix(10, 0)
		clientA := identityClient("env-a--zone-a", "service", &realmA, created)
		clientB := identityClient("env-b--zone-b", "service", &realmB, created)
		clientOther := identityClient("env-b--zone-b", "different", &realmB, time.Unix(0, 0))
		reader := indexedReader(clientA, clientB, clientOther)

		peers, err := ClientsForRealmGroup(context.Background(), reader, []identityv1.Realm{realmB, realmA, realmB}, clientB)

		Expect(err).NotTo(HaveOccurred())
		Expect(peers).To(HaveLen(2))
		Expect(ElectClient(peers)).To(HaveField("Namespace", "env-a--zone-a"))
	})
})

func indexedReader(objects ...client.Object) client.Reader {
	scheme := runtime.NewScheme()
	Expect(identityv1.AddToScheme(scheme)).To(Succeed())

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&identityv1.IdentityProvider{}, IndexFieldSpecAdminURL, AdminURLIndexValue).
		WithIndex(&identityv1.Realm{}, IndexFieldSpecIdentityProvider, func(obj client.Object) []string {
			realm := obj.(*identityv1.Realm)
			if realm.Spec.IdentityProvider == nil {
				return nil
			}
			return []string{realm.Spec.IdentityProvider.String()}
		}).
		WithIndex(&identityv1.Client{}, IndexFieldSpecRealm, func(obj client.Object) []string {
			identityClient := obj.(*identityv1.Client)
			if identityClient.Spec.Realm == nil {
				return nil
			}
			return []string{identityClient.Spec.Realm.String()}
		}).
		WithObjects(objects...).
		Build()
}

func identityProvider(namespace, name, adminURL string) *identityv1.IdentityProvider {
	return &identityv1.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       identityv1.IdentityProviderSpec{AdminUrl: adminURL},
	}
}

func realm(namespace string, idp *identityv1.IdentityProvider, created time.Time) *identityv1.Realm {
	return &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "rover",
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec: identityv1.RealmSpec{IdentityProvider: types.ObjectRefFromObject(idp)},
	}
}

func identityClient(namespace, name string, realm *identityv1.Realm, created time.Time) *identityv1.Client {
	c := &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec: identityv1.ClientSpec{ClientId: name},
	}
	if realm != nil {
		c.Spec.Realm = types.ObjectRefFromObject(realm)
	}
	return c
}
