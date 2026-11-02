// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	"github.com/telekom/controlplane/identity/internal/leader"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("shared leadership watch mappings", func() {
	const (
		sharedURL = "https://keycloak.example.com/auth"
		otherURL  = "https://other.example.com/auth"
	)

	var (
		ctx    context.Context
		reader client.Client

		roverA        *identityv1.Realm
		roverB        *identityv1.Realm
		defaultRealmA *identityv1.Realm
		roverOther    *identityv1.Realm
		idpA          *identityv1.IdentityProvider

		gatewayA      *identityv1.Client
		gatewayB      *identityv1.Client
		fileB         *identityv1.Client
		gatewayOther  *identityv1.Client
		defaultClient *identityv1.Client
	)

	BeforeEach(func() {
		ctx = context.Background()

		idpA = mappingIdentityProvider("env-a--zone", sharedURL)
		idpB := mappingIdentityProvider("env-b--zone", "HTTPS://KEYCLOAK.example.com:443/auth/")
		idpOther := mappingIdentityProvider("env-c--zone", otherURL)

		roverA = mappingRealm("env-a--zone", "rover", idpA)
		roverB = mappingRealm("env-b--zone", "rover", idpB)
		defaultRealmA = mappingRealm("env-a--zone", "env-a", idpA)
		roverOther = mappingRealm("env-c--zone", "rover", idpOther)

		gatewayA = mappingClient("rover", roverA)
		gatewayB = mappingClient("rover", roverB)
		fileB = mappingClient("env-b--sftp-api", roverB)
		gatewayOther = mappingClient("rover", roverOther)
		defaultClient = mappingClient("rover", defaultRealmA)
		defaultClient.Name = "default-rover"

		reader = mappingReader(idpA, idpB, idpOther, roverA, roverB, defaultRealmA, roverOther,
			gatewayA, gatewayB, fileB, gatewayOther, defaultClient)
	})

	It("maps an IdentityProvider to every Realm of IdentityProviders with the same admin URL", func() {
		r := &RealmReconciler{Client: reader}

		Expect(r.mapIdpObjToRealm(ctx, idpA)).To(ConsistOf(requestsFor(roverA, roverB, defaultRealmA)))
	})

	It("maps a Realm to the Realms of its shared leader-election group", func() {
		r := &RealmReconciler{Client: reader}

		Expect(r.mapRealmObjToRealm(ctx, roverB)).To(ConsistOf(requestsFor(roverA, roverB)))
	})

	It("maps a Realm to all Clients in the Realms of its shared group", func() {
		r := &ClientReconciler{Client: reader}

		Expect(r.mapRealmObjToIdentityClient(ctx, roverA)).To(ConsistOf(requestsFor(gatewayA, gatewayB, fileB)))
	})

	It("maps a Client to the Clients with the same client ID in the shared Realms", func() {
		r := &ClientReconciler{Client: reader}

		Expect(r.mapClientObjToIdentityClient(ctx, gatewayB)).To(ConsistOf(requestsFor(gatewayA, gatewayB)))
	})

	It("ignores a Client without a Realm", func() {
		r := &ClientReconciler{Client: reader}
		orphan := &identityv1.Client{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "env-a--zone"}}

		Expect(r.mapClientObjToIdentityClient(ctx, orphan)).To(BeEmpty())
	})
})

func mappingIdentityProvider(namespace, adminURL string) *identityv1.IdentityProvider {
	return &identityv1.IdentityProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "idp", Namespace: namespace},
		Spec:       identityv1.IdentityProviderSpec{AdminUrl: adminURL},
	}
}

func mappingRealm(namespace, name string, idp *identityv1.IdentityProvider) *identityv1.Realm {
	return &identityv1.Realm{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       identityv1.RealmSpec{IdentityProvider: types.ObjectRefFromObject(idp)},
	}
}

func mappingClient(clientID string, realm *identityv1.Realm) *identityv1.Client {
	return &identityv1.Client{
		ObjectMeta: metav1.ObjectMeta{Name: clientID, Namespace: realm.Namespace},
		Spec:       identityv1.ClientSpec{ClientId: clientID, Realm: types.ObjectRefFromObject(realm)},
	}
}

func requestsFor(objects ...client.Object) []reconcile.Request {
	requests := make([]reconcile.Request, 0, len(objects))
	for _, obj := range objects {
		requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
	}
	return requests
}

// mappingReader returns a fake client with the field indexes registered by RegisterIndexesOrDie.
func mappingReader(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	Expect(identityv1.AddToScheme(scheme)).To(Succeed())

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&identityv1.IdentityProvider{}, leader.IndexFieldSpecAdminURL, leader.AdminURLIndexValue).
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
