// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared_test

import (
	"github.com/telekom/controlplane/projector/internal/domain/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("StatusFromConditions", func() {
	It("returns READY when Ready condition is True", func() {
		conditions := []metav1.Condition{
			{
				Type:    "Ready",
				Status:  metav1.ConditionTrue,
				Message: "all good",
			},
		}
		phase, msg := shared.StatusFromConditions(conditions)
		Expect(phase).To(Equal("READY"))
		Expect(msg).To(Equal("all good"))
	})

	It("returns ERROR when Ready condition is False with reason Error", func() {
		conditions := []metav1.Condition{
			{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "Error",
				Message: "something broke",
			},
		}
		phase, msg := shared.StatusFromConditions(conditions)
		Expect(phase).To(Equal("ERROR"))
		Expect(msg).To(Equal("something broke"))
	})

	It("returns ERROR when Ready condition is False with reason Failed", func() {
		conditions := []metav1.Condition{
			{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "Failed",
				Message: "permanently failed",
			},
		}
		phase, msg := shared.StatusFromConditions(conditions)
		Expect(phase).To(Equal("ERROR"))
		Expect(msg).To(Equal("permanently failed"))
	})

	It("returns PENDING when Ready condition is False with other reason", func() {
		conditions := []metav1.Condition{
			{
				Type:    "Ready",
				Status:  metav1.ConditionFalse,
				Reason:  "Provisioning",
				Message: "setting up",
			},
		}
		phase, msg := shared.StatusFromConditions(conditions)
		Expect(phase).To(Equal("PENDING"))
		Expect(msg).To(Equal("setting up"))
	})

	It("returns UNKNOWN when Ready condition is missing", func() {
		conditions := []metav1.Condition{
			{
				Type:   "Available",
				Status: metav1.ConditionTrue,
			},
		}
		phase, msg := shared.StatusFromConditions(conditions)
		Expect(phase).To(Equal("UNKNOWN"))
		Expect(msg).To(BeEmpty())
	})

	It("returns UNKNOWN for nil conditions", func() {
		phase, msg := shared.StatusFromConditions(nil)
		Expect(phase).To(Equal("UNKNOWN"))
		Expect(msg).To(BeEmpty())
	})

	It("returns UNKNOWN for empty conditions slice", func() {
		phase, msg := shared.StatusFromConditions([]metav1.Condition{})
		Expect(phase).To(Equal("UNKNOWN"))
		Expect(msg).To(BeEmpty())
	})

	It("returns UNKNOWN for unrecognized condition status", func() {
		conditions := []metav1.Condition{
			{
				Type:    "Ready",
				Status:  metav1.ConditionStatus("SomethingElse"),
				Message: "weird state",
			},
		}
		phase, msg := shared.StatusFromConditions(conditions)
		Expect(phase).To(Equal("UNKNOWN"))
		Expect(msg).To(Equal("weird state"))
	})
})
