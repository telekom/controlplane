// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/rover-server/internal/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MapResponse", func() {
	Context("with valid conditions", func() {
		It("returns complete response when processing is done and ready is true", func() {
			// Test with processing condition = Done and ready condition = True
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Status: metav1.ConditionFalse,
					Reason: "Done",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
				},
			}

			response, err := MapResponse(conditions)

			// Verify the response
			Expect(err).NotTo(HaveOccurred())
			Expect(response.State).To(Equal(api.Complete))
			Expect(response.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(response.OverallStatus).To(Equal(api.OverallStatusComplete))
		})
	})

	Context("with blocked conditions", func() {
		It("returns blocked response when processing is blocked", func() {
			// Test with processing condition = Blocked
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeProcessing,
					Status:  metav1.ConditionFalse,
					Reason:  "Blocked",
					Message: "Resource is blocked",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionFalse,
				},
			}

			response, err := MapResponse(conditions)

			// Verify the response
			Expect(err).NotTo(HaveOccurred())
			Expect(response.State).To(Equal(api.Blocked))
			Expect(response.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(response.OverallStatus).To(Equal(api.OverallStatusBlocked))
		})
	})

	Context("with empty conditions", func() {
		It("returns None status for all fields", func() {
			// Test with empty conditions
			var conditions []metav1.Condition

			response, err := MapResponse(conditions)

			// Verify the response
			Expect(err).NotTo(HaveOccurred())
			Expect(response.State).To(Equal(api.None))
			Expect(response.ProcessingState).To(Equal(api.ProcessingStateNone))
			Expect(response.OverallStatus).To(Equal(api.OverallStatusNone))
		})
	})
})
