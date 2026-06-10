// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApprovalExpiration Controller", Ordered, func() {
	const resourceName = "approval-expiration-lifecycle"

	namespacedName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}
	approvalExpirationName := types.NamespacedName{Name: resourceName, Namespace: testNamespace}

	requester := approvalv1.Requester{
		TeamName:  "test--requester",
		TeamEmail: "requester@example.com",
		Reason:    "Need access for testing",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "requester-app-name",
				Namespace: testNamespace,
			},
		},
	}

	decider := approvalv1.Decider{
		TeamName:  "test--decider",
		TeamEmail: "decider@example.com",
		ApplicationRef: &ctypes.TypedObjectRef{
			TypeMeta: metav1.TypeMeta{},
			ObjectRef: ctypes.ObjectRef{
				Name:      "decider-app-name",
				Namespace: testNamespace,
			},
		},
	}

	target := ctypes.TypedObjectRef{
		TypeMeta: metav1.TypeMeta{Kind: "Subscription"},
		ObjectRef: ctypes.ObjectRef{
			Name:      resourceName,
			Namespace: testNamespace,
		},
	}

	var firstExpiration metav1.Time
	var recreatedExpiration metav1.Time

	BeforeAll(func() {
		approval := &approvalv1.Approval{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: testNamespace,
				Labels: map[string]string{
					config.EnvironmentLabelKey: testEnvironment,
				},
			},
			Spec: approvalv1.ApprovalSpec{
				Action:    "subscribe",
				Target:    target,
				Requester: requester,
				Decider:   decider,
				Strategy:  approvalv1.ApprovalStrategySimple,
				State:     approvalv1.ApprovalStateGranted,
				Decisions: []approvalv1.Decision{{
					Name:           "Alice Approver",
					Email:          "alice@example.com",
					Comment:        "Initial approval",
					ResultingState: approvalv1.ApprovalStateGranted,
				}},
			},
		}

		Expect(k8sClient.Create(ctx, approval)).To(Succeed())
	})

	AfterAll(func() {
		approval := &approvalv1.Approval{}
		err := k8sClient.Get(ctx, namespacedName, approval)
		if apierrors.IsNotFound(err) {
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Delete(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, namespacedName, &approvalv1.Approval{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())

		expiration := &approvalv1.ApprovalExpiration{}
		err = k8sClient.Get(ctx, approvalExpirationName, expiration)
		if err == nil {
			Expect(k8sClient.Delete(ctx, expiration)).To(Succeed())
		} else {
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}
	})

	It("creates an ApprovalExpiration for a granted approval", func() {
		Eventually(func(g Gomega) {
			approval := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, approval)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(approval.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			g.Expect(approval.Status.ExpiresAt).NotTo(BeNil())

			expiration := &approvalv1.ApprovalExpiration{}
			err = k8sClient.Get(ctx, approvalExpirationName, expiration)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(expiration.Spec.Approval.Name).To(Equal(resourceName))
			g.Expect(expiration.Spec.Approval.Namespace).To(Equal(testNamespace))
			g.Expect(approval.Status.ExpiresAt.Time).To(Equal(expiration.Spec.Expiration.Time))

			firstExpiration = expiration.Spec.Expiration
		}, timeout, interval).Should(Succeed())
	})

	It("deletes ApprovalExpiration and clears ExpiresAt when approval is rejected", func() {
		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, namespacedName, approval)).To(Succeed())
		approval.Spec.State = approvalv1.ApprovalStateRejected
		approval.Spec.Decisions = []approvalv1.Decision{{
			Name:           "Bob Decider",
			Email:          "bob@example.com",
			Comment:        "Rejecting access",
			ResultingState: approvalv1.ApprovalStateRejected,
		}}
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Spec.State).To(Equal(approvalv1.ApprovalStateRejected))
			g.Expect(updated.Status.ExpiresAt).To(BeNil())
		}, timeout, interval).Should(Succeed())

		Eventually(func(g Gomega) {
			err := k8sClient.Get(ctx, approvalExpirationName, &approvalv1.ApprovalExpiration{})
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
	})

	It("recreates ApprovalExpiration with a new expiration when approval is granted again", func() {
		By("waiting long enough for the recreated expiration timestamp to move forward")
		time.Sleep(1100 * time.Millisecond)

		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, namespacedName, approval)).To(Succeed())
		approval.Spec.State = approvalv1.ApprovalStateGranted
		approval.Spec.Decisions = []approvalv1.Decision{{
			Name:           "Carol Decider",
			Email:          "carol@example.com",
			Comment:        "Granting access again",
			ResultingState: approvalv1.ApprovalStateGranted,
		}}
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Spec.State).To(Equal(approvalv1.ApprovalStateGranted))
			g.Expect(updated.Status.ExpiresAt).NotTo(BeNil())

			expiration := &approvalv1.ApprovalExpiration{}
			err = k8sClient.Get(ctx, approvalExpirationName, expiration)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Status.ExpiresAt.Time).To(Equal(expiration.Spec.Expiration.Time))
			g.Expect(expiration.Spec.Expiration.Time.After(firstExpiration.Time)).To(BeTrue())

			recreatedExpiration = expiration.Spec.Expiration
		}, timeout, interval).Should(Succeed())
	})

	It("keeps the same expiration when approval is suspended", func() {
		approval := &approvalv1.Approval{}
		Expect(k8sClient.Get(ctx, namespacedName, approval)).To(Succeed())
		approval.Spec.State = approvalv1.ApprovalStateSuspended
		approval.Spec.Decisions = []approvalv1.Decision{{
			Name:           "Dana Decider",
			Email:          "dana@example.com",
			Comment:        "Suspending access",
			ResultingState: approvalv1.ApprovalStateSuspended,
		}}
		Expect(k8sClient.Update(ctx, approval)).To(Succeed())

		Eventually(func(g Gomega) {
			updated := &approvalv1.Approval{}
			err := k8sClient.Get(ctx, namespacedName, updated)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(updated.Spec.State).To(Equal(approvalv1.ApprovalStateSuspended))
			g.Expect(updated.Status.ExpiresAt).NotTo(BeNil())
			g.Expect(updated.Status.ExpiresAt.Time).To(Equal(recreatedExpiration.Time))

			expiration := &approvalv1.ApprovalExpiration{}
			err = k8sClient.Get(ctx, approvalExpirationName, expiration)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(expiration.Spec.Expiration.Time).To(Equal(recreatedExpiration.Time))
		}, timeout, interval).Should(Succeed())
	})
})
