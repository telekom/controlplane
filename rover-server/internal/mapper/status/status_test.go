// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Rover Status Mapper", func() {
	Context("MapRoverStatus", func() {
		It("must map rover status correctly", func() {
			status := MapRoverStatus(ctx, rover)

			Expect(status).ToNot(BeNil())

			snaps.MatchJSON(GinkgoT(), status)
		})

	})

	Context("MapRoverResponse", func() {
		It("must map rover response correctly", func() {
			response, err := MapRoverResponse(ctx, rover)

			Expect(response).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), response)

			Expect(err).To(BeNil())
		})

		It("must return an error if the input rover is nil", func() {
			response, err := MapRoverResponse(ctx, nil)

			Expect(response).ToNot(BeNil())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

		It("must map rover response correctly when processing condition is missing", func() {
			roverNoProcessing := &v1.Rover{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rover.cp.ei.telekom.de/v1",
					Kind:       "Rover",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rover-not-processed",
					Namespace: "poc--eni--hyperion",
				},
			}

			response, err := MapRoverResponse(ctx, roverNoProcessing)

			Expect(response).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), response)

			Expect(err).To(BeNil())
		})
	})

	Context("GetOverallStatus", func() {
		It("must get overall status correctly", func() {
			status := GetOverallStatus(rover.GetConditions())

			Expect(status).ToNot(BeNil())
			Expect(status).To(Equal(api.OverallStatusBlocked))
		})

		It("must get overall status correctly if the input is nil", func() {
			status := GetOverallStatus(nil)

			Expect(status).ToNot(BeNil())
			Expect(status).To(Equal(api.OverallStatusNone))
		})
	})

	Context("FillStateInfo", func() {
		It("if no processing condition exists status will be none and warnings are created", func() {
			var conditions []metav1.Condition
			status := &api.Status{}

			fillStateInfo(conditions, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

		It("if no ready condition exists status will be none and warnings are created", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Status: metav1.ConditionTrue,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

		It("if processing condition status will be blocked and processing", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Status: metav1.ConditionTrue,
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionFalse,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

		It("if processing condition is blocked status will be blocked and warning", func() {
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeProcessing,
					Reason:  "Blocked",
					Message: "Blocked due to dependency",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionFalse,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

		It("if processing done and ready true status will be complete", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Reason: "Done",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

		It("if processing done and ready false status will be blocked and warning", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Reason: "Done",
				},
				{
					Type:    condition.ConditionTypeReady,
					Status:  metav1.ConditionFalse,
					Message: "Not ready yet",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

		It("if processing failed status will be failed and error", func() {
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeProcessing,
					Reason:  "Failed",
					Message: "Processing failed",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionFalse,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

	})
})
