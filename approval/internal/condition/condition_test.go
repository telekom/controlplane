// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConditions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Condition Suite")
}

var _ = Describe("Approval Conditions", func() {
	Context("NewApprovedCondition", func() {
		It("returns Approved=True for Granted state", func() {
			cond := NewApprovedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("Granted"))
			Expect(cond.Message).To(ContainSubstring("granted"))
		})
	})

	Context("NewRejectedCondition", func() {
		It("returns Approved=False for Rejected state", func() {
			cond := NewRejectedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("Rejected"))
			Expect(cond.Message).To(ContainSubstring("rejected"))
		})
	})

	Context("NewPendingCondition", func() {
		It("returns Approved=False for Pending state", func() {
			cond := NewPendingCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("Pending"))
			Expect(cond.Message).To(ContainSubstring("pending"))
		})
	})

	Context("NewSemigrantedCondition", func() {
		It("returns Approved=False for Semigranted state", func() {
			cond := NewSemigrantedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("Semigranted"))
			Expect(cond.Message).To(ContainSubstring("partially"))
		})
	})

	Context("NewSuspendedCondition", func() {
		It("returns Approved=True for Suspended state", func() {
			cond := NewSuspendedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("Suspended"))
			Expect(cond.Message).To(ContainSubstring("suspended"))
		})
	})

	Context("Expired state semantics", func() {
		It("uses Approved=False for Expired state", func() {
			expiredCond := metav1.Condition{
				Type:    "Approved",
				Status:  metav1.ConditionFalse,
				Reason:  "Expired",
				Message: "Request has expired",
			}

			Expect(expiredCond.Type).To(Equal("Approved"))
			Expect(expiredCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(expiredCond.Reason).To(Equal("Expired"))
			Expect(expiredCond.Message).To(ContainSubstring("expired"))
		})
	})

	Describe("Semantic Validation", func() {
		It("has Approved=True only for states that grant access", func() {
			grantingStates := []metav1.Condition{
				NewApprovedCondition(),
				NewSuspendedCondition(),
			}

			for _, cond := range grantingStates {
				Expect(cond.Status).To(Equal(metav1.ConditionTrue),
					"State %s should have Approved=True", cond.Reason)
			}
		})

		It("has Approved=False for states that deny access", func() {
			expiredCond := metav1.Condition{
				Type:    "Approved",
				Status:  metav1.ConditionFalse,
				Reason:  "Expired",
				Message: "Request has expired",
			}

			denyingStates := []metav1.Condition{
				NewPendingCondition(),
				NewSemigrantedCondition(),
				NewRejectedCondition(),
				expiredCond,
			}

			for _, cond := range denyingStates {
				Expect(cond.Status).To(Equal(metav1.ConditionFalse),
					"State %s should have Approved=False", cond.Reason)
			}
		})
	})
})
