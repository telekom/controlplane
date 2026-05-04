// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	commonconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApprovalExpiration Controller", Ordered, func() {
	const resourceName = "test-expiration-approval"

	typeNamespacedName := types.NamespacedName{
		Name:      resourceName,
		Namespace: testNamespace,
	}

	decider := approvalv1.Decider{
		TeamName:  "test--decider",
		TeamEmail: "test@decider.com",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "decider-app-name",
				Namespace: testNamespace,
				UID:       "",
			},
		},
	}

	requester := approvalv1.Requester{
		TeamName:  "test--requester",
		TeamEmail: "max.mustermann@telekom.de",
		Reason:    "I need access to this API",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "requester-app-name",
				Namespace: testNamespace,
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
			Namespace: testNamespace,
		},
	}

	BeforeAll(func() {
		By("creating the Approval in GRANTED state")
		approval := &approvalv1.Approval{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "approval.cp.ei.telekom.de/v1",
				Kind:       "Approval",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					commonconfig.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: approvalv1.ApprovalSpec{
				Strategy:  approvalv1.ApprovalStrategySimple,
				State:     approvalv1.ApprovalStateGranted,
				Requester: requester,
				Target:    resource,
				Decider:   decider,
				Action:    "subscribe",
				Decisions: []approvalv1.Decision{
					{
						Name:           "Alice",
						Email:          "alice@telekom.de",
						Comment:        "Approved",
						ResultingState: approvalv1.ApprovalStateGranted,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, typeNamespacedName, approval)
			g.Expect(err).NotTo(HaveOccurred())
			readyCondition := meta.FindStatusCondition(approval.Status.Conditions, condition.ConditionTypeReady)
			g.Expect(readyCondition).ToNot(BeNil())
			g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		}, timeout, interval).Should(Succeed())
	})

	AfterAll(func() {
		By("cleaning up test resources")
		approval := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, typeNamespacedName, approval)
		if err == nil {
			Expect(k8sClient.Delete(ctx, approval)).To(Succeed())
		}

		expirationName := types.NamespacedName{
			Name:      resourceName + "--expiration",
			Namespace: testNamespace,
		}
		ae := &approvalv1.ApprovalExpiration{}
		err = k8sClient.Get(ctx, expirationName, ae)
		if err == nil {
			Expect(k8sClient.Delete(ctx, ae)).To(Succeed())
		}
	})

	It("should create ApprovalExpiration when Approval is GRANTED", func() {
		expirationName := types.NamespacedName{
			Name:      resourceName + "--expiration",
			Namespace: testNamespace,
		}

		Eventually(func(g Gomega) {
			ae := &approvalv1.ApprovalExpiration{}
			err := k8sClient.Get(ctx, expirationName, ae)
			g.Expect(err).NotTo(HaveOccurred())

			By("checking ApprovalExpiration has correct owner reference")
			g.Expect(ae.OwnerReferences).To(HaveLen(1))
			g.Expect(ae.OwnerReferences[0].Name).To(Equal(resourceName))
			g.Expect(ae.OwnerReferences[0].Kind).To(Equal("Approval"))
			g.Expect(*ae.OwnerReferences[0].Controller).To(BeTrue())

			By("checking ApprovalExpiration has expiration dates set")
			g.Expect(ae.Spec.Expiration.Time).To(BeTemporally(">", time.Now()))
			g.Expect(ae.Spec.WeeklyReminder.Time).To(BeTemporally("<", ae.Spec.Expiration.Time))
			g.Expect(ae.Spec.DailyReminder.Time).To(BeTemporally("<", ae.Spec.Expiration.Time))
			g.Expect(ae.Spec.DailyReminder.Time).To(BeTemporally(">", ae.Spec.WeeklyReminder.Time))

			By("checking ApprovalExpiration references the Approval")
			g.Expect(ae.Spec.Approval.Name).To(Equal(resourceName))
			g.Expect(ae.Spec.Approval.Namespace).To(Equal(testNamespace))
		}, timeout, interval).Should(Succeed())
	})

	It("should delete ApprovalExpiration when Approval transitions to REJECTED", func() {
		By("transitioning Approval to REJECTED")
		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, approval)).To(Succeed())

		approval.Spec.State = approvalv1.ApprovalStateRejected
		approval.Spec.Decisions = append(approval.Spec.Decisions, approvalv1.Decision{
			Name:           "Bob",
			Email:          "bob@telekom.de",
			Comment:        "Rejected",
			ResultingState: approvalv1.ApprovalStateRejected,
		})
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		expirationName := types.NamespacedName{
			Name:      resourceName + "--expiration",
			Namespace: testNamespace,
		}

		By("checking ApprovalExpiration is deleted")
		Eventually(func(g Gomega) {
			ae := &approvalv1.ApprovalExpiration{}
			err := k8sClient.Get(ctx, expirationName, ae)
			g.Expect(errors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("should recreate ApprovalExpiration when Approval transitions back to GRANTED", func() {
		By("transitioning Approval back to GRANTED")
		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, approval)).To(Succeed())

		approval.Spec.State = approvalv1.ApprovalStateGranted
		approval.Spec.Decisions = append(approval.Spec.Decisions, approvalv1.Decision{
			Name:           "Charlie",
			Email:          "charlie@telekom.de",
			Comment:        "Re-approved",
			ResultingState: approvalv1.ApprovalStateGranted,
		})
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		expirationName := types.NamespacedName{
			Name:      resourceName + "--expiration",
			Namespace: testNamespace,
		}

		By("checking ApprovalExpiration is recreated")
		Eventually(func(g Gomega) {
			ae := &approvalv1.ApprovalExpiration{}
			err := k8sClient.Get(ctx, expirationName, ae)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ae.Spec.Expiration.Time).To(BeTemporally(">", time.Now()))
		}, timeout, interval).Should(Succeed())
	})

	It("should keep ApprovalExpiration when Approval transitions to SUSPENDED", func() {
		expirationName := types.NamespacedName{
			Name:      resourceName + "--expiration",
			Namespace: testNamespace,
		}

		By("getting current expiration time before SUSPENDED")
		ae := &approvalv1.ApprovalExpiration{}
		Expect(k8sClient.Get(ctx, expirationName, ae)).To(Succeed())
		originalExpiration := ae.Spec.Expiration.Time

		By("transitioning Approval to SUSPENDED")
		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, approval)).To(Succeed())

		approval.Spec.State = approvalv1.ApprovalStateSuspended
		approval.Spec.Decisions = append(approval.Spec.Decisions, approvalv1.Decision{
			Name:           "Dave",
			Email:          "dave@telekom.de",
			Comment:        "Suspended",
			ResultingState: approvalv1.ApprovalStateSuspended,
		})
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		By("checking ApprovalExpiration still exists with same expiration time")
		Consistently(func(g Gomega) {
			ae := &approvalv1.ApprovalExpiration{}
			err := k8sClient.Get(ctx, expirationName, ae)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ae.Spec.Expiration.Time).To(Equal(originalExpiration))
		}, 2*time.Second, interval).Should(Succeed())
	})
})
