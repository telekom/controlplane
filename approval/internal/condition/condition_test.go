// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConditions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Condition Suite")
}

var _ = Describe("Approval Conditions", func() {
	// The "Approved" condition type should indicate whether the approval grants access
	// Status=True means approved and access is granted
	// Status=False means not approved or access is revoked

	Context("NewApprovedCondition", func() {
		It("should return Approved=True for Granted state", func() {
			cond := NewApprovedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("Granted"))
			Expect(cond.Message).To(ContainSubstring("granted"))
		})
	})

	Context("NewRejectedCondition", func() {
		It("should return Approved=False for Rejected state", func() {
			cond := NewRejectedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("Rejected"))
			Expect(cond.Message).To(ContainSubstring("rejected"))
		})
	})

	Context("NewPendingCondition", func() {
		It("should return Approved=False for Pending state", func() {
			cond := NewPendingCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("Pending"))
			Expect(cond.Message).To(ContainSubstring("pending"))
		})
	})

	Context("NewSemigrantedCondition", func() {
		It("should return Approved=False for Semigranted state", func() {
			cond := NewSemigrantedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("Semigranted"))
			Expect(cond.Message).To(ContainSubstring("partially"))
		})
	})

	Context("NewSuspendedCondition", func() {
		It("should return Approved=True for Suspended state", func() {
			cond := NewSuspendedCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("Suspended"))
			Expect(cond.Message).To(ContainSubstring("suspended"))
		})
	})

	Context("NewExpiredCondition", func() {
		It("should return Approved=False for Expired state", func() {
			cond := NewExpiredCondition()
			Expect(cond.Type).To(Equal("Approved"))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse), "Expired approvals should not be considered approved")
			Expect(cond.Reason).To(Equal("Expired"))
			Expect(cond.Message).To(ContainSubstring("expired"))
		})
	})

	// Semantic validation: states that grant access vs states that don't
	Describe("Semantic Validation", func() {
		It("should have Approved=True only for states that grant access", func() {
			// States that GRANT access
			grantingStates := []metav1.Condition{
				NewApprovedCondition(),  // Granted
				NewSuspendedCondition(), // Suspended (debatable, but currently True)
			}

			for _, cond := range grantingStates {
				Expect(cond.Status).To(Equal(metav1.ConditionTrue),
					"State %s should have Approved=True", cond.Reason)
			}
		})

		It("should have Approved=False for states that deny access", func() {
			// States that DENY or DON'T YET grant access
			denyingStates := []metav1.Condition{
				NewPendingCondition(),     // Not yet approved
				NewSemigrantedCondition(), // Partially approved (not enough)
				NewRejectedCondition(),    // Explicitly denied
				NewExpiredCondition(),     // No longer approved
			}

			for _, cond := range denyingStates {
				Expect(cond.Status).To(Equal(metav1.ConditionFalse),
					"State %s should have Approved=False", cond.Reason)
			}
		})
	})
})
