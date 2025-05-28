// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ghErrors "github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/condition"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	identityproviderModel "github.com/telekom/controlplane/identity/internal/model/identityprovider"
	realmModel "github.com/telekom/controlplane/identity/internal/model/realm"
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
