// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("Rover Status Mapper", func() {
	Context("MapRoverStatus", func() {
		It("must map rover status correctly", func() {
			status, err := MapRoverStatus(ctx, rover, stores)

			Expect(err).To(BeNil())
			Expect(status).ToNot(BeNil())
			// Rover fixture: Ready=False/SubResourceNotReady, sub-resource blocked (NoApproval)
			// reconcileWithSubResources overrides to Blocked/Done
			Expect(status.State).To(Equal(api.Blocked))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
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

			status, err := MapRoverStatus(ctx, completeRover, stores)

			Expect(err).To(BeNil())
			Expect(status.State).To(Equal(api.Complete))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
		})
	})

	Context("MapRoverResponse", func() {
		It("must map rover response correctly", func() {
			response, err := MapRoverResponse(ctx, rover, stores)

			Expect(err).To(BeNil())
			Expect(response).ToNot(BeNil())
			// Rover fixture: Ready=False/SubResourceNotReady, sub-resource blocked (NoApproval)
			// reconcileWithSubResources overrides parent to Blocked/Done
			// CompareAndReturn(Blocked, Blocked) → Blocked
			Expect(response.State).To(Equal(api.Blocked))
			Expect(response.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(response.OverallStatus).To(Equal(api.OverallStatusBlocked))
		})

		It("must return an error if the input rover is nil", func() {
			response, err := MapRoverResponse(ctx, nil, stores)

			Expect(response).ToNot(BeNil())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

		It("must map rover response correctly when no conditions exist", func() {
			roverNoConditions := &v1.Rover{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "rover.cp.ei.telekom.de/v1",
					Kind:       "Rover",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rover-not-processed",
					Namespace: "poc--eni--hyperion",
				},
			}

			response, err := MapRoverResponse(ctx, roverNoConditions, stores)

			Expect(err).To(BeNil())
			Expect(response).ToNot(BeNil())
			Expect(response.State).To(Equal(api.None))
			Expect(response.ProcessingState).To(Equal(api.ProcessingStatePending))
			Expect(response.OverallStatus).To(Equal(api.OverallStatusPending))
		})
	})

	Context("GetOverallStatus", func() {
		It("must get overall status correctly", func() {
			status := GetOverallStatus(rover.GetConditions())

			Expect(status).ToNot(BeNil())
			Expect(status).To(Equal(api.OverallStatusProcessing))
		})

		It("must get overall status correctly if the input is nil", func() {
			status := GetOverallStatus(nil)

			Expect(status).ToNot(BeNil())
			Expect(status).To(Equal(api.OverallStatusPending))
		})
	})

	Context("FillStateInfo", func() {
		It("if no conditions exist status will be none with warning", func() {
			var conditions []metav1.Condition
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.None))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStatePending))
			Expect(status.Warnings).To(HaveLen(1))
			Expect(status.Warnings[0].Message).To(Equal("No conditions found"))
		})

		It("if only processing exists (no ready) falls back to processing condition", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Status: metav1.ConditionTrue,
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.None))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateProcessing))
		})

		It("if ready is false with processing-equivalent reason returns processing", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Status: metav1.ConditionTrue,
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionFalse,
					Reason: "SubResourceNotReady",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.None))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateProcessing))
		})

		It("if ready is false with Provisioning reason returns processing", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Status: metav1.ConditionTrue,
					Reason: "Provisioning",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionFalse,
					Reason: "Provisioning",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.None))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateProcessing))
		})

		It("if ready is false with non-processing reason returns blocked", func() {
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeProcessing,
					Reason:  "Blocked",
					Message: "Blocked due to dependency",
				},
				{
					Type:    condition.ConditionTypeReady,
					Status:  metav1.ConditionFalse,
					Reason:  "ApprovalPending",
					Message: "Waiting for approval",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Blocked))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(status.Warnings).To(HaveLen(1))
			Expect(status.Warnings[0].Message).To(Equal("Waiting for approval"))
		})

		It("if ready is true status will be complete", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Reason: "Done",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Provisioned",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Complete))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
		})

		It("if ready is true then ready wins even if processing says blocked", func() {
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeProcessing,
					Status:  metav1.ConditionFalse,
					Reason:  "Blocked",
					Message: "Zone is not ready yet",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: "Provisioned",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Complete))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
		})

		It("if ready is unknown and processing is blocked falls back to processing", func() {
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeProcessing,
					Status:  metav1.ConditionFalse,
					Reason:  "Blocked",
					Message: "Environment label is missing",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionUnknown,
					Reason: "Unknown",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Blocked))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(status.Warnings).To(HaveLen(1))
			Expect(status.Warnings[0].Message).To(Equal("Environment label is missing"))
		})

		It("if ready is unknown and processing is done falls back to processing", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeProcessing,
					Status: metav1.ConditionFalse,
					Reason: "Done",
				},
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionUnknown,
					Reason: "Unknown",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.None))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
		})

		It("if processing has unknown reason and ready is false with unknown reason returns blocked", func() {
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeProcessing,
					Status:  metav1.ConditionFalse,
					Reason:  "Failed",
					Message: "Processing failed",
				},
				{
					Type:    condition.ConditionTypeReady,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeFailure",
					Message: "Something went wrong",
				},
			}
			status := &api.Status{}

			fillStateInfo(conditions, 0, status)

			Expect(status).ToNot(BeNil())
			Expect(status.State).To(Equal(api.Blocked))
			Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(status.Warnings).To(HaveLen(1))
			Expect(status.Warnings[0].Message).To(Equal("Something went wrong"))
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

		It("if ready condition is stale status will be none and pending", func() {
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

var _ = Describe("MapAPISpecificationStatus", func() {
	It("returns Complete/Done when conditions are complete and no sub-resource is stale", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-ok", Namespace: "ns", UID: "uid-as1", Generation: 2},
			Status: v1.ApiSpecificationStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 2},
				},
			},
		}
		s := &store.Stores{}

		status, err := MapAPISpecificationStatus(ctx, apiSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(api.Complete))
		Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
	})

	It("returns Processing when complete but Api sub-resource is stale", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-stale", Namespace: "ns", UID: "uid-as2", Generation: 2},
			Status: v1.ApiSpecificationStatus{
				Api: types.ObjectRef{Name: "api-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 2},
				},
			},
		}

		apiMock := new(MockObjectStore[*apiv1.Api])
		staleAPI := &apiv1.Api{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "ns", Generation: 5},
			Status: apiv1.ApiStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
				},
			},
		}
		apiMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{staleAPI}}, nil)

		s := &store.Stores{APIStore: apiMock}

		status, err := MapAPISpecificationStatus(ctx, apiSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(api.Complete))
		Expect(status.ProcessingState).To(Equal(api.ProcessingStateProcessing))
	})

	It("returns error when staleness check fails", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-err", Namespace: "ns", UID: "uid-as3", Generation: 2},
			Status: v1.ApiSpecificationStatus{
				Api: types.ObjectRef{Name: "api-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 2},
				},
			},
		}

		apiMock := new(MockObjectStore[*apiv1.Api])
		apiMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*apiv1.Api])(nil), fmt.Errorf("staleness error"))

		s := &store.Stores{APIStore: apiMock}

		_, err := MapAPISpecificationStatus(ctx, apiSpec, s)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("staleness error"))
	})

	It("appends state infos when state is not Complete", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-blocked", Namespace: "ns", UID: "uid-as4", Generation: 2},
			Status: v1.ApiSpecificationStatus{
				Api: types.ObjectRef{Name: "api-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Message: "Not ready yet", ObservedGeneration: 2},
				},
			},
		}

		// Api sub-resource is also not ready
		apiMock := new(MockObjectStore[*apiv1.Api])
		notReadyAPI := &apiv1.Api{
			TypeMeta:   metav1.TypeMeta{APIVersion: "api.cp.ei.telekom.de/v1", Kind: "Api"},
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "ns", Generation: 2},
			Status: apiv1.ApiStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "NotReady", Message: "Api not ready"},
				},
			},
		}
		apiMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{notReadyAPI}}, nil)

		s := &store.Stores{APIStore: apiMock}

		status, err := MapAPISpecificationStatus(ctx, apiSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(api.Blocked))
		Expect(status.Errors).NotTo(BeEmpty())
	})

	It("returns error when state info retrieval fails", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-err2", Namespace: "ns", UID: "uid-as5", Generation: 2},
			Status: v1.ApiSpecificationStatus{
				Api: types.ObjectRef{Name: "api-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Message: "Not ready", ObservedGeneration: 2},
				},
			},
		}

		apiMock := new(MockObjectStore[*apiv1.Api])
		apiMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*apiv1.Api])(nil), fmt.Errorf("state info error"))

		s := &store.Stores{APIStore: apiMock}

		_, err := MapAPISpecificationStatus(ctx, apiSpec, s)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("state info error"))
	})
})

var _ = Describe("MapEventSpecificationStatus", func() {
	It("returns Complete/Done when conditions are complete and no sub-resource is stale", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-ok", Namespace: "ns", UID: "uid-es1", Generation: 2},
			Status: v1.EventSpecificationStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 2},
				},
			},
		}
		s := &store.Stores{}

		status, err := MapEventSpecificationStatus(ctx, eventSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(api.Complete))
		Expect(status.ProcessingState).To(Equal(api.ProcessingStateDone))
	})

	It("returns Processing when complete but EventType sub-resource is stale", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-stale", Namespace: "ns", UID: "uid-es2", Generation: 2},
			Status: v1.EventSpecificationStatus{
				EventType: types.ObjectRef{Name: "et-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 2},
				},
			},
		}

		etMock := new(MockObjectStore[*eventv1.EventType])
		staleET := &eventv1.EventType{
			ObjectMeta: metav1.ObjectMeta{Name: "et-1", Namespace: "ns", Generation: 5},
			Status: eventv1.EventTypeStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
				},
			},
		}
		etMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{staleET}}, nil)

		s := &store.Stores{EventTypeStore: etMock}

		status, err := MapEventSpecificationStatus(ctx, eventSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(api.Complete))
		Expect(status.ProcessingState).To(Equal(api.ProcessingStateProcessing))
	})

	It("returns error when staleness check fails", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-err", Namespace: "ns", UID: "uid-es3", Generation: 2},
			Status: v1.EventSpecificationStatus{
				EventType: types.ObjectRef{Name: "et-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 2},
				},
			},
		}

		etMock := new(MockObjectStore[*eventv1.EventType])
		etMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*eventv1.EventType])(nil), fmt.Errorf("staleness error"))

		s := &store.Stores{EventTypeStore: etMock}

		_, err := MapEventSpecificationStatus(ctx, eventSpec, s)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("staleness error"))
	})

	It("appends state infos when state is not Complete", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-blocked", Namespace: "ns", UID: "uid-es4", Generation: 2},
			Status: v1.EventSpecificationStatus{
				EventType: types.ObjectRef{Name: "et-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Message: "Not ready yet", ObservedGeneration: 2},
				},
			},
		}

		etMock := new(MockObjectStore[*eventv1.EventType])
		notReadyET := &eventv1.EventType{
			TypeMeta:   metav1.TypeMeta{APIVersion: "event.cp.ei.telekom.de/v1", Kind: "EventType"},
			ObjectMeta: metav1.ObjectMeta{Name: "et-1", Namespace: "ns", Generation: 2},
			Status: eventv1.EventTypeStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "NotReady", Message: "EventType not ready"},
				},
			},
		}
		etMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{notReadyET}}, nil)

		s := &store.Stores{EventTypeStore: etMock}

		status, err := MapEventSpecificationStatus(ctx, eventSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(status.State).To(Equal(api.Blocked))
		Expect(status.Errors).NotTo(BeEmpty())
	})

	It("returns error when state info retrieval fails", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-err2", Namespace: "ns", UID: "uid-es5", Generation: 2},
			Status: v1.EventSpecificationStatus{
				EventType: types.ObjectRef{Name: "et-1", Namespace: "ns"},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Message: "Not ready", ObservedGeneration: 2},
				},
			},
		}

		etMock := new(MockObjectStore[*eventv1.EventType])
		etMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*eventv1.EventType])(nil), fmt.Errorf("state info error"))

		s := &store.Stores{EventTypeStore: etMock}

		_, err := MapEventSpecificationStatus(ctx, eventSpec, s)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("state info error"))
	})
})

var _ = Describe("MapRoverStatus additional paths", func() {
	It("returns error when staleness check fails", func() {
		r := &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "r-stale-err", Namespace: "ns", UID: "uid-r1", Generation: 2},
			Status: v1.RoverStatus{
				ApiSubscriptions: []types.ObjectRef{{Name: "sub-1", Namespace: "ns"}},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 2},
				},
			},
		}

		apiSubMock := new(MockObjectStore[*apiv1.ApiSubscription])
		apiSubMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*apiv1.ApiSubscription])(nil), fmt.Errorf("staleness check error"))

		s := &store.Stores{APISubscriptionStore: apiSubMock}

		_, err := MapRoverStatus(ctx, r, s)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("staleness check error"))
	})

	It("returns error when state info retrieval fails", func() {
		r := &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "r-info-err", Namespace: "ns", UID: "uid-r2", Generation: 2},
			Status: v1.RoverStatus{
				ApiSubscriptions: []types.ObjectRef{{Name: "sub-1", Namespace: "ns"}},
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
					{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Message: "Not ready", ObservedGeneration: 2},
				},
			},
		}

		apiSubMock := new(MockObjectStore[*apiv1.ApiSubscription])
		apiSubMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*apiv1.ApiSubscription])(nil), fmt.Errorf("state info error"))

		s := &store.Stores{APISubscriptionStore: apiSubMock}

		_, err := MapRoverStatus(ctx, r, s)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("state info error"))
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
