// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCondition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Condition Suite")
}

var _ = Describe("Condition Tests", func() {

	Context("EnsureReady function", func() {
		It("should return an error", func() {
			obj := test.NewObject("fake", "default")

			err := EnsureReady(obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("TestResource 'default/fake' is not ready"))
		})

		It("should return nil", func() {
			obj := test.NewObject("fake", "default")
			obj.SetCondition(NewReadyCondition("TrustMe", "ImReady"))

			err := EnsureReady(obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Constructor functions", func() {

		It("should return a new BlockedCondition", func() {
			condition := NewBlockedCondition("Blocked")
			Expect(condition.Type).To(Equal(ConditionTypeProcessing))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Blocked"))
			Expect(condition.Message).To(Equal("Blocked"))
		})

		It("should return a new ProcessingCondition", func() {
			condition := NewProcessingCondition("Reason", "Processing")
			Expect(condition.Type).To(Equal(ConditionTypeProcessing))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("Reason"))
			Expect(condition.Message).To(Equal("Processing"))
		})

		It("should return a new DoneProcessingCondition", func() {
			condition := NewDoneProcessingCondition("Done")
			Expect(condition.Type).To(Equal(ConditionTypeProcessing))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Done"))
			Expect(condition.Message).To(Equal("Done"))
		})

		It("should return a new ReadyCondition", func() {
			condition := NewReadyCondition("Reason", "Ready")
			Expect(condition.Type).To(Equal(ConditionTypeReady))
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("Reason"))
			Expect(condition.Message).To(Equal("Ready"))
		})

		It("should return a new NotReadyCondition", func() {
			condition := NewNotReadyCondition("Reason", "NotReady")
			Expect(condition.Type).To(Equal(ConditionTypeReady))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Reason"))
			Expect(condition.Message).To(Equal("NotReady"))
		})

		It("should return a new UnknownCondition", func() {
			condition := SetToUnknown(ReadyCondition)
			Expect(condition.Type).To(Equal(ConditionTypeReady))
			Expect(condition.Status).To(Equal(metav1.ConditionUnknown))
			Expect(condition.Reason).To(Equal("Unknown"))
			Expect(condition.Message).To(Equal(""))
		})

	})
})
