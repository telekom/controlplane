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
			status, err := MapRoverStatus(ctx, rover)

			Expect(err).To(BeNil())
			Expect(status).ToNot(BeNil())

			snaps.MatchJSON(GinkgoT(), status)
		})

		It("stays Complete/Done when sub-resources have no ObservedGeneration (backward compat)", func() {
			completeRover := &v1.Rover{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rover.cp.ei.telekom.de/v1",
					Kind:       "Rover",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "rover-local-sub",
					Namespace:  "poc--eni--hyperion",
					UID:        "549badcc-18b6-48ac-a8ce-b3242523d827",
					Generation: 2,
				},
				Status: v1.RoverStatus{
					Conditions: []metav1.Condition{
						{
							Type:               condition.ConditionTypeProcessing,
							Status:             metav1.ConditionFalse,
							Reason:             "Done",
							ObservedGeneration: 2,
						},
						{
							Type:               condition.ConditionTypeReady,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 2,
						},
					},
				},
			}

			status, err := MapRoverStatus(ctx, completeRover)

			Expect(err).To(BeNil())
			Expect(status.State).To(Equal(api.Complete))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
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

	Context("isProcessingStale", func() {
		It("returns true when processing ObservedGeneration is behind object generation", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 3,
				},
			}

			Expect(isProcessingStale(conditions, 5)).To(BeTrue())
		})

		It("returns false when processing ObservedGeneration matches object generation", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 5,
				},
			}

			Expect(isProcessingStale(conditions, 5)).To(BeFalse())
		})

		It("returns false when there is no processing condition", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
				},
			}

			Expect(isProcessingStale(conditions, 5)).To(BeFalse())
		})

		It("returns false when ObservedGeneration is zero (backward compat)", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 0,
				},
			}

			Expect(isProcessingStale(conditions, 5)).To(BeFalse())
		})

		It("returns false when object generation is zero", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 3,
				},
			}

			Expect(isProcessingStale(conditions, 0)).To(BeFalse())
		})
	})

	Context("FillStateInfo", func() {
		It("if no processing condition exists status will be none and warnings are created", func() {
			var conditions []metav1.Condition
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

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

			fillStateInfo(conditions, 0, status)

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

			fillStateInfo(conditions, 0, status)

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

			fillStateInfo(conditions, 0, status)

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

			fillStateInfo(conditions, 0, status)

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

			fillStateInfo(conditions, 0, status)

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

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), status)
		})

		It("if processing condition is stale status will be none and pending", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 1,
				},
				{
					Type:               condition.ConditionTypeReady,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 2, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.None))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStatePending))
		})

		It("if processing condition is current status follows normal evaluation", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 2,
				},
				{
					Type:               condition.ConditionTypeReady,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 2,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 2, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Complete))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
		})

		It("if observedGeneration is zero staleness detection is skipped (backward compat)", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 0,
				},
				{
					Type:               condition.ConditionTypeReady,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 0,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 5, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Complete))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
		})

		It("if objectGeneration is zero staleness detection is skipped", func() {
			conditions := []metav1.Condition{
				{
					Type:               condition.ConditionTypeProcessing,
					Status:             metav1.ConditionFalse,
					Reason:             "Done",
					ObservedGeneration: 1,
				},
				{
					Type:               condition.ConditionTypeReady,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Complete))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
		})

	})
})

var _ = Describe("CalculateOverallStatus", func() {
	Context("when processing state is Processing", func() {
		It("returns Processing", func() {
			result := CalculateOverallStatus(api.Complete, api.ProcessingStateProcessing)
			Expect(result).To(Equal(api.OverallStatusProcessing))
		})
	})

	Context("when processing state is Failed", func() {
		It("returns Failed", func() {
			result := CalculateOverallStatus(api.Complete, api.ProcessingStateFailed)
			Expect(result).To(Equal(api.OverallStatusFailed))
		})
	})

	Context("when state is Blocked", func() {
		It("returns Blocked", func() {
			result := CalculateOverallStatus(api.Blocked, api.ProcessingStatePending)
			Expect(result).To(Equal(api.OverallStatusBlocked))
		})
	})

	Context("when state is Complete and processing state is Done", func() {
		It("returns Complete", func() {
			result := CalculateOverallStatus(api.Complete, api.ProcessingStateDone)
			Expect(result).To(Equal(api.OverallStatusComplete))
		})
	})

	Context("when state is unknown", func() {
		It("returns None", func() {
			result := CalculateOverallStatus("unknown", api.ProcessingStateNone)
			Expect(result).To(Equal(api.OverallStatusNone))
		})
	})
})

var _ = DescribeTable("CompareAndReturn",
	func(a, b, expected api.OverallStatus) {
		Expect(CompareAndReturn(a, b)).To(Equal(expected))
	},
	Entry("Failed takes precedence over Processing", api.OverallStatusProcessing, api.OverallStatusFailed, api.OverallStatusFailed),
	Entry("Blocked takes precedence over Processing", api.OverallStatusProcessing, api.OverallStatusBlocked, api.OverallStatusBlocked),
	Entry("Processing takes precedence over Pending", api.OverallStatusPending, api.OverallStatusProcessing, api.OverallStatusProcessing),
	Entry("Complete vs Complete returns Complete", api.OverallStatusComplete, api.OverallStatusComplete, api.OverallStatusComplete),
	Entry("None vs None returns None", api.OverallStatusNone, api.OverallStatusNone, api.OverallStatusNone),
	Entry("Invalid takes precedence over Failed", api.OverallStatusInvalid, api.OverallStatusFailed, api.OverallStatusInvalid),
	Entry("Done vs Complete returns first when equal priority", api.OverallStatusDone, api.OverallStatusComplete, api.OverallStatusDone),
	Entry("Processing takes precedence over Done", api.OverallStatusDone, api.OverallStatusProcessing, api.OverallStatusProcessing),
	Entry("Complete takes precedence over unknown", api.OverallStatus("unknown"), api.OverallStatusComplete, api.OverallStatusComplete),
)
