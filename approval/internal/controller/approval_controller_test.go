// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
)

var _ = Describe("Approval Controller", Ordered, func() {

	const resourceName = "test-resource"

	ctx := context.Background()

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: "default", // TODO(user):Modify as needed
	}
	approval := &approvalv1.Approval{}

	decider := approvalv1.Decider{
		TeamName:  "test--decider",
		TeamEmail: "test@decider.com",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "decider-app-name",
				Namespace: "default",
				UID:       "",
			},
		},
	}

	requester := approvalv1.Requester{
		TeamName:  "test--requester",
		TeamEmail: "max.mustermann@telekom.de",
		Reason:    "I need access to this API!!",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "requester-app-name",
				Namespace: "default",
				UID:       "",
			},
		},
	}

	resource := ctypes.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{
			Kind: "Subscription",
		},
		ObjectRef: ctypes.ObjectRef{
			Name:      resourceName,
			Namespace: "default",
		},
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
					Target:    resource,
					Decider:   decider,
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
			g.Expect(approval.Spec.Requester.TeamName).To(BeEquivalentTo("test--requester"))

		}, timeout, interval).Should(Succeed())

	})

	It("should successfully reconcile the granted approval", func() {
		By("Granted")
		checkApprovalStatus(typeNamespacedName, approvalv1.ApprovalStateGranted,
			metav1.ConditionFalse, metav1.ConditionTrue,
			"Approval granted", "Approval has been granted",
			"Done", "Approved")

		By("Checking the notifications")
		var grantedApproval = &approvalv1.Approval{}
		err := k8sClient.Get(ctx, typeNamespacedName, grantedApproval)
		Expect(err).NotTo(HaveOccurred())

		Expect(grantedApproval.Status.NotificationRefs).NotTo(BeNil())
		Expect(grantedApproval.Status.NotificationRefs).To(HaveLen(2))

		By("Validating the decider notification")
		deciderNotificationRef := types.NamespacedName{
			Name:      "approval--subscription--updated--decider--test-resource--559f5f87c",
			Namespace: "default",
		}

		deciderNotification := &notificationv1.Notification{}
		err = k8sClient.Get(ctx, deciderNotificationRef, deciderNotification)
		Expect(err).NotTo(HaveOccurred())
		Expect(deciderNotification.Spec.Purpose).To(Equal("approval--subscription--updated--decider"))
		Expect(deciderNotification.Spec.Properties).NotTo(BeNil())

		By("Validating the requester notification")
		requesterNotificationRef := types.NamespacedName{
			Name:      "approval--subscription--updated--requester--test-resource--7f57689449",
			Namespace: "default",
		}

		requesterNotification := &notificationv1.Notification{}
		err = k8sClient.Get(ctx, requesterNotificationRef, requesterNotification)
		Expect(err).NotTo(HaveOccurred())
		Expect(requesterNotification.Spec.Purpose).To(Equal("approval--subscription--updated--requester"))
		Expect(requesterNotification.Spec.Properties).NotTo(BeNil())
		ExpectJSONEqual(requesterNotification.Spec.Properties.Raw, []byte(`{ "requester_team": "requester", "scopes": "read", "state_new": "Granted", "decider_application": "decider-app-name", "decider_group": "test", "environment": "test", "requester_application": "requester-app-name", "requester_group": "test", "state_old": "Pending", "basepath": "/eni/distr/v1", "decider_team": "decider" }`))
	})

	It("should successfully reconcile the rejected approval", func() {
		By("Rejected")
		checkApprovalStatus(typeNamespacedName, approvalv1.ApprovalStateRejected,
			metav1.ConditionFalse, metav1.ConditionFalse,
			"Approval rejected", "Approval has been rejected",
			"Done", "Rejected")

	})

	It("should successfully reconcile the suspended approval", func() {
		By("Suspended")
		checkApprovalStatus(typeNamespacedName, approvalv1.ApprovalStateSuspended,
			metav1.ConditionTrue, metav1.ConditionTrue,
			"Approval is suspended", "Approval is suspended",
			"Suspended", "Suspended")

	})
})

func checkApprovalStatus(typeNamespacedName types.NamespacedName, state approvalv1.ApprovalState,
	expectedProcessingStatus, expectedReadyStatus metav1.ConditionStatus,
	expectedProcessingMessage, expectedReadyMessage,
	expectedProcessingReason, expectedReadyReason string,
) {
	fetchedApproval := &approvalv1.Approval{}
	err := k8sClient.Get(ctx, typeNamespacedName, fetchedApproval)
	Expect(err).NotTo(HaveOccurred())

	fetchedApproval.Spec.State = state

	// Update Approval
	Expect(k8sClient.Update(ctx, fetchedApproval)).Should(Succeed())

	fetchedUpdatedApproval := &approvalv1.Approval{}

	Eventually(func(g Gomega) {
		err = k8sClient.Get(ctx, typeNamespacedName, fetchedUpdatedApproval)
		g.Expect(err).NotTo(HaveOccurred())

		processingCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeProcessing)
		readyCondition := meta.FindStatusCondition(fetchedUpdatedApproval.Status.Conditions, condition.ConditionTypeReady)

		g.Expect(processingCondition).ToNot(BeNil())
		g.Expect(processingCondition.Reason).To(Equal(expectedProcessingReason))
		g.Expect(processingCondition.Status).To(Equal(expectedProcessingStatus))
		g.Expect(processingCondition.Message).To(Equal(expectedProcessingMessage))

		g.Expect(readyCondition).ToNot(BeNil())
		g.Expect(readyCondition.Reason).To(Equal(expectedReadyReason))
		g.Expect(readyCondition.Status).To(Equal(expectedReadyStatus))
		g.Expect(readyCondition.Message).To(Equal(expectedReadyMessage))

	}, timeout, interval).Should(Succeed())
}

func ExpectJSONEqual(actualJSON, expectedJSON []byte) {
	var actual, expected map[string]interface{}
	Expect(json.Unmarshal(actualJSON, &actual)).To(Succeed())
	Expect(json.Unmarshal(expectedJSON, &expected)).To(Succeed())
	Expect(actual).To(Equal(expected))
}
