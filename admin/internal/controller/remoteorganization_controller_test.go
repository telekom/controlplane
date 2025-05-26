// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	"github.com/telekom/controlplane/common/pkg/condition"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
)

var _ = Describe("RemoteOrganization Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := client.ObjectKey{
			Name:      resourceName,
			Namespace: testNamespace,
		}
		testRemoteorganization := &adminv1.RemoteOrganization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: adminv1.RemoteOrganizationSpec{
				Id:           "test-id",
				Url:          "test-url",
				ClientId:     "test-clientId",
				ClientSecret: "topsecret",
				IssuerUrl:    "test-issuerUrl",
				Zone:         types.ObjectRef{Name: "test-zone", Namespace: testEnvironment},
			},
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind RemoteOrganization")
			resource := &adminv1.RemoteOrganization{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, testRemoteorganization)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &adminv1.RemoteOrganization{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance RemoteOrganization")
			Expect(k8sClient.Delete(ctx, testRemoteorganization)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			Eventually(func(g Gomega) {
				VerifyRemoteOrganization(ctx, g, typeNamespacedName, testRemoteorganization)

				expectedNamespaceName := "test--test-id"
				VerifyNamespace(ctx, g, expectedNamespaceName)

			}, timeout, interval).Should(Succeed())
		})
	})
})

func VerifyRemoteOrganization(ctx context.Context, g Gomega, namespacedName client.ObjectKey, remoteOrgToVerify *adminv1.RemoteOrganization) {
	By("Checking if the RemoteOrganization is created and all conditions are set")
	remoteOrg := &adminv1.RemoteOrganization{}
	err := k8sClient.Get(ctx, namespacedName, remoteOrg)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(remoteOrg.Spec).To(Equal(remoteOrgToVerify.Spec))

	g.Expect(remoteOrg.Status.Conditions).To(HaveLen(2))

	g.Expect(meta.IsStatusConditionTrue(remoteOrg.Status.Conditions, condition.ConditionTypeProcessing)).To(BeFalse())
	g.Expect(meta.IsStatusConditionTrue(remoteOrg.Status.Conditions, condition.ConditionTypeReady)).To(BeTrue())
}
