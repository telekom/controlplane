// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"time"

	ghErrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/telekom/controlplane/common/pkg/condition"
	common "github.com/telekom/controlplane/common/pkg/types"
	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	clientModel "github.com/telekom/controlplane/identity/internal/testutil/fixtures/client"
	identityproviderModel "github.com/telekom/controlplane/identity/internal/testutil/fixtures/identityprovider"
	realmModel "github.com/telekom/controlplane/identity/internal/testutil/fixtures/realm"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Realm Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		// IDP related
		realmIdpName := "keycloak-test-realm"
		realmIdpRef := client.ObjectKey{
			Name:      realmIdpName,
			Namespace: testNamespace,
		}
		realmIdp := identityproviderModel.NewIdentityProvider(realmIdpName, testNamespace, testEnvironment)

		// Realm related
		realmName := "test-realm"
		realmRef := client.ObjectKey{
			Name:      realmName,
			Namespace: testNamespace,
		}
		testRealm := realmModel.NewRealm(realmName, testNamespace, testEnvironment, realmIdpName)

		expectedRealmStatus := identityv1.RealmStatus{
			IssuerUrl:     "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/test-realm",
			AdminClientId: "admin-cli",
			AdminUserName: "admin",
			AdminPassword: "password",
			AdminUrl:      "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/admin/realms/",
			AdminTokenUrl: "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/master/protocol/openid-connect/token",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind IdentityProvider")
			NewIdentityProvider(ctx, realmIdpRef, realmIdp)

			By("creating the custom resource for the Kind Realm")
			NewRealm(ctx, realmRef, testRealm)
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance Realm")
			DeleteRealm(ctx, realmRef)

			By("deleting the custom resource for the Kind IdentityProvider")
			DeleteIdentityProvider(ctx, realmIdpRef)
		})
		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifyRealm(ctx, g, realmRef, testRealm, expectedRealmStatus)
			}, timeout, interval).Should(Succeed())
		})
	})
})

var _ = Describe("Realm deletion", func() {
	It("keeps a referenced Realm terminating until its Client is gone", func() {
		ctx := context.Background()
		idpRef := client.ObjectKey{Name: "deletion-guard-idp", Namespace: testNamespace}
		realmRef := client.ObjectKey{Name: "deletion-guard-realm", Namespace: testNamespace}
		clientRef := client.ObjectKey{Name: "deletion-guard-client", Namespace: testNamespace}

		idp := identityproviderModel.NewIdentityProvider(idpRef.Name, idpRef.Namespace, testEnvironment)
		realm := realmModel.NewRealm(realmRef.Name, realmRef.Namespace, testEnvironment, idpRef.Name)
		identityClient := clientModel.NewClient(clientRef.Name, clientRef.Namespace, testEnvironment, realmRef.Name)

		Expect(k8sClient.Create(ctx, idp)).To(Succeed())
		Expect(k8sClient.Create(ctx, realm)).To(Succeed())
		VerifyRealmIsAvailable(realmRef)
		Expect(k8sClient.Create(ctx, identityClient)).To(Succeed())

		Expect(k8sClient.Delete(ctx, realm)).To(Succeed())
		Eventually(func(g Gomega) {
			current := &identityv1.Realm{}
			g.Expect(k8sClient.Get(ctx, realmRef, current)).To(Succeed())
			g.Expect(current.DeletionTimestamp.IsZero()).To(BeFalse())
		}, timeout, interval).Should(Succeed())
		Consistently(func() bool {
			return errors.IsNotFound(k8sClient.Get(ctx, realmRef, &identityv1.Realm{}))
		}, time.Second, interval).Should(BeFalse())

		Expect(k8sClient.Delete(ctx, identityClient)).To(Succeed())
		Eventually(func() bool {
			return errors.IsNotFound(k8sClient.Get(ctx, clientRef, &identityv1.Client{}))
		}, timeout, interval).Should(BeTrue())
		Eventually(func() bool {
			return errors.IsNotFound(k8sClient.Get(ctx, realmRef, &identityv1.Realm{}))
		}, timeout, interval).Should(BeTrue())

		Expect(k8sClient.Delete(ctx, idp)).To(Succeed())
	})
})

var _ = Describe("Realm reference mapping", func() {
	It("ignores empty references", func() {
		identityClient := &identityv1.Client{Spec: identityv1.ClientSpec{Realm: &common.ObjectRef{}}}
		Expect((&RealmReconciler{}).mapClientToRealm(context.Background(), identityClient)).To(BeEmpty())
	})

	It("ignores a Client deletion referencing an active Realm", func() {
		realm := realmModel.NewRealm("active-realm", testNamespace, testEnvironment, "idp")
		reconciler := &RealmReconciler{Client: fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(realm).Build()}
		identityClient := &identityv1.Client{Spec: identityv1.ClientSpec{Realm: common.ObjectRefFromObject(realm)}}

		Expect(reconciler.mapClientToRealm(context.Background(), identityClient)).To(BeEmpty())
	})

	It("maps a Client deletion referencing a terminating Realm", func() {
		deletedAt := metav1.Now()
		realm := realmModel.NewRealm("terminating-realm", testNamespace, testEnvironment, "idp")
		realm.DeletionTimestamp = &deletedAt
		realm.Finalizers = []string{"test-finalizer"}
		reconciler := &RealmReconciler{Client: fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(realm).Build()}
		identityClient := &identityv1.Client{Spec: identityv1.ClientSpec{Realm: common.ObjectRefFromObject(realm)}}

		Expect(reconciler.mapClientToRealm(context.Background(), identityClient)).To(ConsistOf(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(realm)}))
	})
})

//nolint:gocritic // Test helper compares the full expected status structure.
func VerifyRealm(ctx context.Context, gomega Gomega, namespacedName client.ObjectKey, realmToVerify *identityv1.Realm, expectedRealmStatus identityv1.RealmStatus) {
	realmResource := &identityv1.Realm{}
	err := k8sClient.Get(ctx, namespacedName, realmResource)

	gomega.Expect(err).NotTo(HaveOccurred())

	gomega.Expect(realmResource.Spec).To(Equal(realmToVerify.Spec))
	gomega.Expect(realmResource.Status.Conditions).To(HaveLen(2))
	gomega.Expect(realmResource.Status.IssuerUrl).To(Equal(expectedRealmStatus.IssuerUrl))
	gomega.Expect(realmResource.Status.AdminClientId).To(Equal(expectedRealmStatus.AdminClientId))
	gomega.Expect(realmResource.Status.AdminUserName).To(Equal(expectedRealmStatus.AdminUserName))
	gomega.Expect(realmResource.Status.AdminPassword).To(Equal(expectedRealmStatus.AdminPassword))
	gomega.Expect(realmResource.Status.AdminUrl).To(Equal(expectedRealmStatus.AdminUrl))
	gomega.Expect(realmResource.Status.AdminTokenUrl).To(Equal(expectedRealmStatus.AdminTokenUrl))
	gomega.Expect(meta.IsStatusConditionTrue(realmResource.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	gomega.Expect(meta.IsStatusConditionTrue(realmResource.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}

func VerifyRealmIsAvailable(clientRealmRef client.ObjectKey) {
	Eventually(func() error {
		return GetRealm(ctx, clientRealmRef)
	}, timeout, interval).Should(Succeed())
}

func GetRealm(ctx context.Context, namespacedName client.ObjectKey) error {
	realmResource := &identityv1.Realm{}
	err := k8sClient.Get(ctx, namespacedName, realmResource)
	if err != nil {
		return err
	}
	if realmResource.Status.IssuerUrl == "" {
		return ghErrors.New("Realm not ready yet. IssuerUrl is empty.")
	}
	return nil
}

func NewRealm(ctx context.Context, namespacedName client.ObjectKey, realm *identityv1.Realm) {
	realmResource := &identityv1.Realm{}
	err := k8sClient.Get(ctx, namespacedName, realmResource)
	if err != nil && errors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, realm)).To(Succeed())
	}
}

func DeleteRealm(ctx context.Context, namespacedName client.ObjectKey) {
	realmResource := &identityv1.Realm{}
	err := k8sClient.Get(ctx, namespacedName, realmResource)
	Expect(err).NotTo(HaveOccurred())

	Expect(k8sClient.Delete(ctx, realmResource)).To(Succeed())
}
