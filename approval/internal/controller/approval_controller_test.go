// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
)

var _ = Describe("ApiSubscription Controller", Ordered, func() {

	const resourceName = "test-resource"

	ctx := context.Background()

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: "default", // TODO(user):Modify as needed
	}
	approval := &approvalv1.Approval{}

	requester := approvalv1.Requester{
		Name:   "Max",
		Email:  "max.mustermann@telekom.de",
		Reason: "I need access to this API!!",
	}

	properties := map[string]any{
		"basePath": "/eni/distr/v1",
		"scopes":   "read",
	}

	err := requester.SetProperties(properties)
	if err != nil {
		return
	}

	BeforeAll(func() {
		By("creating the custom resource for the Kind Approval")
		err := k8sClient.Get(ctx, typeNamespacedName, approval)
		if err != nil && errors.IsNotFound(err) {
			resource := &approvalv1.Approval{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
					Labels: map[string]string{
						config.EnvironmentLabelKey: testEnvironment,
					},
				},
				Spec: approvalv1.ApprovalSpec{
					Strategy:  approvalv1.ApprovalStrategyAuto,
					State:     approvalv1.ApprovalStatePending,
					Requester: requester,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, typeNamespacedName, resource)
				g.Expect(err).NotTo(HaveOccurred())
				processingCondition := meta.FindStatusCondition(resource.Status.Conditions, condition.ConditionTypeProcessing)
				g.Expect(processingCondition).ToNot(BeNil())
			}, timeout, interval).Should(Succeed())
		}
	})

	AfterAll(func() {
		resource := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, typeNamespacedName, resource)
		Expect(err).NotTo(HaveOccurred())

		By("Cleanup the specific resource instance Approval")
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("should successfully reconcile the created resource", func() {

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, typeNamespacedName, approval)

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(approval.Spec.State).To(BeEquivalentTo("Pending"))
			g.Expect(approval.Spec.Strategy).To(BeEquivalentTo("Auto"))
			g.Expect(approval.Spec.Requester.Name).To(BeEquivalentTo("Max"))

		}, timeout, interval).Should(Succeed())

	})

	It("should successfully reconcile the granted approval", func() {

		By("Granted")
		fetchedApproval := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, typeNamespacedName, fetchedApproval)
		Expect(err).NotTo(HaveOccurred())

		fetchedApproval.Spec.State = approvalv1.ApprovalStateGranted

		// Update Approval
		Expect(k8sClient.Update(ctx, fetchedApproval)).Should(Succeed())

		fetchedUpdatedApproval := &approvalv1.Approval{}

		Eventually(func(g Gomega) {
			err = k8sClient.Get(ctx, typeNamespacedName, fetchedUpdatedApproval)
			g.Expect(err).NotTo(HaveOccurred())

			processingCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeProcessing)
			readyCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeReady)

			g.Expect(processingCondition).ToNot(BeNil())
			g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(processingCondition.Message).To(Equal("Approval granted"))

			g.Expect(readyCondition).ToNot(BeNil())
			g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(readyCondition.Message).To(Equal("Approval has been granted"))

		}, timeout, interval).Should(Succeed())

	})

	It("should successfully reconcile the rejected approval", func() {

		By("Rejected")
		fetchedApproval := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, typeNamespacedName, fetchedApproval)
		Expect(err).NotTo(HaveOccurred())

		fetchedApproval.Spec.State = approvalv1.ApprovalStateRejected

		// Update Approval
		Expect(k8sClient.Update(ctx, fetchedApproval)).Should(Succeed())

		fetchedUpdatedApproval := &approvalv1.Approval{}

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, typeNamespacedName, fetchedUpdatedApproval)
			g.Expect(err).NotTo(HaveOccurred())

			processingCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeProcessing)
			readyCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeReady)

			g.Expect(processingCondition).ToNot(BeNil())
			g.Expect(processingCondition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(processingCondition.Message).To(Equal("Approval rejected"))

			g.Expect(readyCondition).ToNot(BeNil())
			g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(readyCondition.Message).To(Equal("Approval has been rejected"))

		}, timeout, interval).Should(Succeed())

	})

	It("should successfully reconcile the suspended approval", func() {

		By("Suspended")
		fetchedApproval := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, typeNamespacedName, fetchedApproval)
		Expect(err).NotTo(HaveOccurred())

		fetchedApproval.Spec.State = approvalv1.ApprovalStateSuspended

		// Update Approval
		Expect(k8sClient.Update(ctx, fetchedApproval)).Should(Succeed())

		fetchedUpdatedApproval := &approvalv1.Approval{}

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, typeNamespacedName, fetchedUpdatedApproval)
			g.Expect(err).NotTo(HaveOccurred())

			processingCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeProcessing)
			readyCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeReady)

			g.Expect(processingCondition).ToNot(BeNil())
			g.Expect(processingCondition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(processingCondition.Reason).To(Equal("Suspended"))
			g.Expect(processingCondition.Message).To(Equal("Approval is suspended"))

			g.Expect(readyCondition).ToNot(BeNil())
			g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(readyCondition.Message).To(Equal("Approval is suspended"))

		}, timeout, interval).Should(Succeed())

	})
})
