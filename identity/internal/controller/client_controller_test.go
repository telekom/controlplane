// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	identityv1 "github.com/telekom/controlplane/identity/api/v1"
	clientModel "github.com/telekom/controlplane/identity/internal/model/client"
	identityproviderModel "github.com/telekom/controlplane/identity/internal/model/identityprovider"
	realmModel "github.com/telekom/controlplane/identity/internal/model/realm"
)

var _ = Describe("Client Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		// IDP related
		clientIdpName := "keycloak-test-realm-client"
		clientIdpRef := k8sclient.ObjectKey{
			Name:      clientIdpName,
			Namespace: testNamespace,
		}
		clientIdp := identityproviderModel.NewIdentityProvider(clientIdpName, testNamespace, testEnvironment)

		// Realm related
		clientRealmName := "realm-test-client"
		clientRealmRef := k8sclient.ObjectKey{
			Name:      clientRealmName,
			Namespace: testNamespace,
		}
		clientRealm := realmModel.NewRealm(clientRealmName, testNamespace, testEnvironment, clientIdpName)

		// Client related
		clientName := "test-client"
		clientRef := k8sclient.ObjectKey{
			Name:      clientName,
			Namespace: testNamespace,
		}
		testClient := clientModel.NewClient(clientName, testNamespace, testEnvironment, clientRealmName)

		BeforeEach(func() {
			By("creating the custom resource for the Kind IdentityProvider")
			NewIdentityProvider(ctx, clientIdpRef, clientIdp)

			By("creating the custom resource for the Kind Realm")
			NewRealm(ctx, clientRealmRef, clientRealm)
			VerifyRealmIsAvailable(clientRealmRef)

			By("creating the custom resource for the Kind Client")
			NewClient(ctx, clientRef, testClient)
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance Client")
			DeleteClient(ctx, clientRef)

			By("Cleanup the specific resource instance Realm")
			DeleteRealm(ctx, clientRealmRef)

			By("deleting the custom resource for the Kind IdentityProvider")
			DeleteIdentityProvider(ctx, clientIdpRef)

		})
		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifyClient(ctx, g, clientRef, testClient)
			}, timeout, interval).Should(Succeed())
		})
	})
})

const expectedIssuerUrl = "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/realm-test-client"

func VerifyClient(ctx context.Context, gomega Gomega, namespacedName k8sclient.ObjectKey, clientToVerify *identityv1.Client) {
	clientResource := &identityv1.Client{}
	err := k8sClient.Get(ctx, namespacedName, clientResource)

	gomega.Expect(err).NotTo(HaveOccurred())

	gomega.Expect(clientResource.Spec).To(Equal(clientToVerify.Spec))
	gomega.Expect(clientResource.Status.IssuerUrl).To(Equal(expectedIssuerUrl))
	gomega.Expect(clientResource.Status.Conditions).To(HaveLen(2))
	gomega.Expect(meta.IsStatusConditionTrue(clientResource.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	gomega.Expect(meta.IsStatusConditionTrue(clientResource.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}

func NewClient(ctx context.Context, namespacedName k8sclient.ObjectKey, client *identityv1.Client) {
	clientResource := &identityv1.Client{}
	err := k8sClient.Get(ctx, namespacedName, clientResource)
	if err != nil && errors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, client)).To(Succeed())
	}
}

func DeleteClient(ctx context.Context, namespacedName k8sclient.ObjectKey) {
	clientResource := &identityv1.Client{}
	err := k8sClient.Get(ctx, namespacedName, clientResource)
	Expect(err).NotTo(HaveOccurred())

	Expect(k8sClient.Delete(ctx, clientResource)).To(Succeed())
}
