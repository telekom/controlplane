// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"fmt"
	"time"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	"github.com/telekom/controlplane/rover-server/internal/api"
	"github.com/telekom/controlplane/rover-server/pkg/store"
	"github.com/telekom/controlplane/rover-server/test/mocks"
	v1 "github.com/telekom/controlplane/rover/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// --- helpers ---

// completeConditions returns Processing=Done + Ready=True conditions with the given observedGeneration.
func completeConditions(gen int64) []metav1.Condition {
	return []metav1.Condition{
		{
			Type:               condition.ConditionTypeProcessing,
			Status:             metav1.ConditionFalse,
			Reason:             "Done",
			Message:            "Provisioned",
			ObservedGeneration: gen,
			LastTransitionTime: metav1.Time{Time: time.Date(2025, 10, 8, 7, 16, 40, 0, time.UTC)},
		},
		{
			Type:               condition.ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gen,
		},
	}
}

// blockedConditions returns Processing=Done + Ready=False (not-complete) conditions.
func blockedConditions(gen int64) []metav1.Condition {
	return []metav1.Condition{
		{
			Type:               condition.ConditionTypeProcessing,
			Status:             metav1.ConditionFalse,
			Reason:             "Done",
			Message:            "Provisioned all sub-resources",
			ObservedGeneration: gen,
			LastTransitionTime: metav1.Time{Time: time.Date(2025, 10, 8, 7, 16, 40, 0, time.UTC)},
		},
		{
			Type:               condition.ConditionTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "SubResourceNotReady",
			Message:            "At least one sub-resource is being processed",
			ObservedGeneration: gen,
		},
	}
}

// --- MapResponse ---

var _ = Describe("Response Mapper", func() {
	Context("MapResponse", func() {
		It("maps a complete resource correctly", func() {
			obj := &v1.Rover{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-rover",
					Namespace:         "ns",
					Generation:        2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 9, 18, 8, 0, 0, 0, time.UTC)},
				},
				Status: v1.RoverStatus{Conditions: completeConditions(2)},
			}

			resp, err := MapResponse(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(api.Complete))
			Expect(resp.ProcessingState).To(Equal(api.ProcessingStateDone))
			Expect(resp.OverallStatus).To(Equal(api.OverallStatusComplete))
			Expect(resp.CreatedAt).To(Equal(time.Date(2025, 9, 18, 8, 0, 0, 0, time.UTC)))
			Expect(resp.ProcessedAt).To(Equal(time.Date(2025, 10, 8, 7, 16, 40, 0, time.UTC)))
		})

		It("returns zero processedAt when processing condition is missing", func() {
			obj := &v1.Rover{
				ObjectMeta: metav1.ObjectMeta{Name: "no-proc", Namespace: "ns"},
			}

			resp, err := MapResponse(ctx, obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.ProcessedAt).To(BeZero())
			Expect(resp.State).To(Equal(api.None))
		})
	})

	// --- MapAPISpecificationResponse ---

	Context("MapAPISpecificationResponse", func() {
		It("must map ApiSpecification response correctly", func() {
			apiSpec := mocks.GetApiSpecification(GinkgoT(), mocks.ApiSpecificationFileName)

			apiStoreMock := mocks.NewAPIStoreMock(GinkgoT())
			s := &store.Stores{APIStore: apiStoreMock}

			resp, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeZero())
			snaps.MatchJSON(GinkgoT(), resp)
		})

		It("returns error for nil input", func() {
			resp, err := MapAPISpecificationResponse(ctx, nil, stores)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("input apiSpec is nil"))
			Expect(resp).To(Equal(api.ResourceStatusResponse{}))
		})

		It("marks processingState as Processing when sub-resource is stale", func() {
			apiSpec := &v1.ApiSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "ApiSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "stale-spec", Namespace: "ns",
					UID: "uid-1", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.ApiSpecificationStatus{
					Conditions: completeConditions(2),
					Api:        types.ObjectRef{Name: "api-1", Namespace: "ns"},
				},
			}

			// Api store returns a stale sub-resource
			apiMock := new(MockObjectStore[*apiv1.Api])
			staleAPI := &apiv1.Api{
				TypeMeta:   metav1.TypeMeta{APIVersion: "api.cp.ei.telekom.de/v1", Kind: "Api"},
				ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "ns", Generation: 5},
				Status: apiv1.ApiStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 3},
					},
				},
			}
			apiMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{staleAPI}}, nil)

			s := &store.Stores{APIStore: apiMock}

			resp, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(api.Complete))
			Expect(resp.ProcessingState).To(Equal(api.ProcessingStateProcessing))
			Expect(resp.OverallStatus).To(Equal(api.OverallStatusProcessing))
		})

		It("collects problems when state is not Complete", func() {
			apiSpec := &v1.ApiSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "ApiSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "blocked-spec", Namespace: "ns",
					UID: "uid-2", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.ApiSpecificationStatus{
					Conditions: blockedConditions(2),
					Api:        types.ObjectRef{Name: "api-1", Namespace: "ns"},
				},
			}

			// Api store returns a sub-resource with a not-ready condition
			apiMock := new(MockObjectStore[*apiv1.Api])
			notReadyAPI := &apiv1.Api{
				TypeMeta:   metav1.TypeMeta{APIVersion: "api.cp.ei.telekom.de/v1", Kind: "Api"},
				ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "ns"},
				Status: apiv1.ApiStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "Failed", Message: "API provisioning failed"},
					},
				},
			}
			apiMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{notReadyAPI}}, nil)

			s := &store.Stores{APIStore: apiMock}

			resp, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(api.Blocked))
			Expect(resp.Errors).To(HaveLen(1))
			Expect(resp.Errors[0].Cause).To(Equal("Failed"))
		})

		It("returns error when staleness check fails", func() {
			apiSpec := &v1.ApiSpecification{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "ApiSpecification"},
				ObjectMeta: metav1.ObjectMeta{Name: "err-spec", Namespace: "ns", UID: "uid-3", Generation: 2},
				Status: v1.ApiSpecificationStatus{
					Conditions: completeConditions(2),
					Api:        types.ObjectRef{Name: "api-1", Namespace: "ns"},
				},
			}

			apiMock := new(MockObjectStore[*apiv1.Api])
			apiMock.On("List", mock.Anything, mock.Anything).Return(
				(*commonStore.ListResponse[*apiv1.Api])(nil), fmt.Errorf("store unavailable"))

			s := &store.Stores{APIStore: apiMock}

			_, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("store unavailable"))
		})

		It("returns error when problems check fails", func() {
			apiSpec := &v1.ApiSpecification{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "ApiSpecification"},
				ObjectMeta: metav1.ObjectMeta{Name: "err-spec2", Namespace: "ns", UID: "uid-4", Generation: 2},
				Status: v1.ApiSpecificationStatus{
					Conditions: blockedConditions(2),
					Api:        types.ObjectRef{Name: "api-1", Namespace: "ns"},
				},
			}

			apiMock := new(MockObjectStore[*apiv1.Api])
			apiMock.On("List", mock.Anything, mock.Anything).Return(
				(*commonStore.ListResponse[*apiv1.Api])(nil), fmt.Errorf("problems store error"))

			s := &store.Stores{APIStore: apiMock}

			_, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("problems store error"))
		})

		It("skips problems when Api ref is empty and state is not Complete", func() {
			apiSpec := &v1.ApiSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "ApiSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-api-ref", Namespace: "ns", UID: "uid-5", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.ApiSpecificationStatus{
					Conditions: blockedConditions(2),
					// Api is zero-value (empty) — problems check short-circuits
				},
			}

			s := &store.Stores{} // no stores needed since Api ref is empty

			resp, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(api.Blocked))
			Expect(resp.Errors).To(BeEmpty())
		})

		It("sets OverallStatus to blocked when parent is Complete but sub-resource is blocked", func() {
			apiSpec := &v1.ApiSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "ApiSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "complete-parent-blocked-sub", Namespace: "ns",
					UID: "uid-6", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.ApiSpecificationStatus{
					Conditions: completeConditions(2),
					Api:        types.ObjectRef{Name: "api-blocked", Namespace: "ns"},
				},
			}

			// Sub-resource is blocked: Processing=Done + Ready=False
			apiMock := new(MockObjectStore[*apiv1.Api])
			blockedAPI := &apiv1.Api{
				TypeMeta:   metav1.TypeMeta{APIVersion: "api.cp.ei.telekom.de/v1", Kind: "Api"},
				ObjectMeta: metav1.ObjectMeta{Name: "api-blocked", Namespace: "ns", Generation: 2},
				Status: apiv1.ApiStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "Blocked", Message: "Upstream unavailable", ObservedGeneration: 2},
					},
				},
			}
			apiMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{blockedAPI}}, nil)

			s := &store.Stores{APIStore: apiMock}

			resp, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).NotTo(HaveOccurred())
			// Parent is Complete/Done, but sub-resource is Blocked → OverallStatus must be "blocked"
			Expect(resp.State).To(Equal(api.Complete))
			Expect(resp.OverallStatus).To(Equal(api.OverallStatusBlocked))
			Expect(resp.Errors).To(HaveLen(1))
			Expect(resp.Errors[0].Cause).To(Equal("Blocked"))
		})

		It("sets OverallStatus to failed when parent is Complete but sub-resource is failed", func() {
			apiSpec := &v1.ApiSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "ApiSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "complete-parent-failed-sub", Namespace: "ns",
					UID: "uid-7", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.ApiSpecificationStatus{
					Conditions: completeConditions(2),
					Api:        types.ObjectRef{Name: "api-failed", Namespace: "ns"},
				},
			}

			// Sub-resource has a failed processing state
			apiMock := new(MockObjectStore[*apiv1.Api])
			failedAPI := &apiv1.Api{
				TypeMeta:   metav1.TypeMeta{APIVersion: "api.cp.ei.telekom.de/v1", Kind: "Api"},
				ObjectMeta: metav1.ObjectMeta{Name: "api-failed", Namespace: "ns", Generation: 2},
				Status: apiv1.ApiStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "ProvisioningError", Message: "Internal server error", ObservedGeneration: 2},
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "ProvisioningError", Message: "Internal server error", ObservedGeneration: 2},
					},
				},
			}
			apiMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{failedAPI}}, nil)

			s := &store.Stores{APIStore: apiMock}

			resp, err := MapAPISpecificationResponse(ctx, apiSpec, s)

			Expect(err).NotTo(HaveOccurred())
			// Parent is Complete/Done, but sub-resource is Failed → OverallStatus must be "failed"
			Expect(resp.State).To(Equal(api.Complete))
			Expect(resp.OverallStatus).To(Equal(api.OverallStatusFailed))
			Expect(resp.Errors).To(HaveLen(1))
		})
	})

	// --- MapEventSpecificationResponse ---

	Context("MapEventSpecificationResponse", func() {
		It("must map EventSpecification response correctly", func() {
			eventSpec := mocks.GetEventSpecification(GinkgoT(), mocks.EventSpecificationFileName)

			eventTypeMock := new(MockObjectStore[*eventv1.EventType])
			eventTypeMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{}}, nil).Maybe()

			s := &store.Stores{EventTypeStore: eventTypeMock}

			resp, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeZero())
			snaps.MatchJSON(GinkgoT(), resp)
		})

		It("returns error for nil input", func() {
			resp, err := MapEventSpecificationResponse(ctx, nil, stores)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("input eventSpec is nil"))
			Expect(resp).To(Equal(api.ResourceStatusResponse{}))
		})

		It("marks processingState as Processing when sub-resource is stale", func() {
			eventSpec := &v1.EventSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "EventSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "stale-event", Namespace: "ns",
					UID: "uid-e1", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.EventSpecificationStatus{
					Conditions: completeConditions(2),
					EventType:  types.ObjectRef{Name: "et-1", Namespace: "ns"},
				},
			}

			etMock := new(MockObjectStore[*eventv1.EventType])
			staleET := &eventv1.EventType{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.cp.ei.telekom.de/v1", Kind: "EventType"},
				ObjectMeta: metav1.ObjectMeta{Name: "et-1", Namespace: "ns", Generation: 5},
				Status: eventv1.EventTypeStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 3},
					},
				},
			}
			etMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{staleET}}, nil)

			s := &store.Stores{EventTypeStore: etMock}

			resp, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(api.Complete))
			Expect(resp.ProcessingState).To(Equal(api.ProcessingStateProcessing))
			Expect(resp.OverallStatus).To(Equal(api.OverallStatusProcessing))
		})

		It("collects problems when state is not Complete", func() {
			eventSpec := &v1.EventSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "EventSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "blocked-event", Namespace: "ns",
					UID: "uid-e2", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.EventSpecificationStatus{
					Conditions: blockedConditions(2),
					EventType:  types.ObjectRef{Name: "et-1", Namespace: "ns"},
				},
			}

			etMock := new(MockObjectStore[*eventv1.EventType])
			notReadyET := &eventv1.EventType{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.cp.ei.telekom.de/v1", Kind: "EventType"},
				ObjectMeta: metav1.ObjectMeta{Name: "et-1", Namespace: "ns"},
				Status: eventv1.EventTypeStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "Failed", Message: "EventType provisioning failed"},
					},
				},
			}
			etMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{notReadyET}}, nil)

			s := &store.Stores{EventTypeStore: etMock}

			resp, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(api.Blocked))
			Expect(resp.Errors).To(HaveLen(1))
			Expect(resp.Errors[0].Cause).To(Equal("Failed"))
		})

		It("returns error when staleness check fails", func() {
			eventSpec := &v1.EventSpecification{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "EventSpecification"},
				ObjectMeta: metav1.ObjectMeta{Name: "err-event", Namespace: "ns", UID: "uid-e3", Generation: 2},
				Status: v1.EventSpecificationStatus{
					Conditions: completeConditions(2),
					EventType:  types.ObjectRef{Name: "et-1", Namespace: "ns"},
				},
			}

			etMock := new(MockObjectStore[*eventv1.EventType])
			etMock.On("List", mock.Anything, mock.Anything).Return(
				(*commonStore.ListResponse[*eventv1.EventType])(nil), fmt.Errorf("event store error"))

			s := &store.Stores{EventTypeStore: etMock}

			_, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("event store error"))
		})

		It("returns error when problems check fails", func() {
			eventSpec := &v1.EventSpecification{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "EventSpecification"},
				ObjectMeta: metav1.ObjectMeta{Name: "err-event2", Namespace: "ns", UID: "uid-e4", Generation: 2},
				Status: v1.EventSpecificationStatus{
					Conditions: blockedConditions(2),
					EventType:  types.ObjectRef{Name: "et-1", Namespace: "ns"},
				},
			}

			etMock := new(MockObjectStore[*eventv1.EventType])
			etMock.On("List", mock.Anything, mock.Anything).Return(
				(*commonStore.ListResponse[*eventv1.EventType])(nil), fmt.Errorf("event problems error"))

			s := &store.Stores{EventTypeStore: etMock}

			_, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("event problems error"))
		})

		It("skips problems when EventType ref is empty and state is not Complete", func() {
			eventSpec := &v1.EventSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "EventSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-et-ref", Namespace: "ns", UID: "uid-e5", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.EventSpecificationStatus{
					Conditions: blockedConditions(2),
					// EventType is zero-value (empty) — problems check short-circuits
				},
			}

			s := &store.Stores{}

			resp, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp.State).To(Equal(api.Blocked))
			Expect(resp.Errors).To(BeEmpty())
		})

		It("sets OverallStatus to blocked when parent is Complete but sub-resource is blocked", func() {
			eventSpec := &v1.EventSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "EventSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "complete-parent-blocked-sub", Namespace: "ns",
					UID: "uid-e6", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.EventSpecificationStatus{
					Conditions: completeConditions(2),
					EventType:  types.ObjectRef{Name: "et-blocked", Namespace: "ns"},
				},
			}

			etMock := new(MockObjectStore[*eventv1.EventType])
			blockedET := &eventv1.EventType{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.cp.ei.telekom.de/v1", Kind: "EventType"},
				ObjectMeta: metav1.ObjectMeta{Name: "et-blocked", Namespace: "ns", Generation: 2},
				Status: eventv1.EventTypeStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "Blocked", Message: "Upstream unavailable", ObservedGeneration: 2},
					},
				},
			}
			etMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{blockedET}}, nil)

			s := &store.Stores{EventTypeStore: etMock}

			resp, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).NotTo(HaveOccurred())
			// Parent is Complete/Done, but sub-resource is Blocked → OverallStatus must be "blocked"
			Expect(resp.State).To(Equal(api.Complete))
			Expect(resp.OverallStatus).To(Equal(api.OverallStatusBlocked))
			Expect(resp.Errors).To(HaveLen(1))
			Expect(resp.Errors[0].Cause).To(Equal("Blocked"))
		})

		It("sets OverallStatus to failed when parent is Complete but sub-resource is failed", func() {
			eventSpec := &v1.EventSpecification{
				TypeMeta: metav1.TypeMeta{APIVersion: "rover.cp.ei.telekom.de/v1", Kind: "EventSpecification"},
				ObjectMeta: metav1.ObjectMeta{
					Name: "complete-parent-failed-sub", Namespace: "ns",
					UID: "uid-e7", Generation: 2,
					CreationTimestamp: metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				Status: v1.EventSpecificationStatus{
					Conditions: completeConditions(2),
					EventType:  types.ObjectRef{Name: "et-failed", Namespace: "ns"},
				},
			}

			etMock := new(MockObjectStore[*eventv1.EventType])
			failedET := &eventv1.EventType{
				TypeMeta:   metav1.TypeMeta{APIVersion: "event.cp.ei.telekom.de/v1", Kind: "EventType"},
				ObjectMeta: metav1.ObjectMeta{Name: "et-failed", Namespace: "ns", Generation: 2},
				Status: eventv1.EventTypeStatus{
					Conditions: []metav1.Condition{
						{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "ProvisioningError", Message: "Internal server error", ObservedGeneration: 2},
						{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "ProvisioningError", Message: "Internal server error", ObservedGeneration: 2},
					},
				},
			}
			etMock.On("List", mock.Anything, mock.Anything).Return(
				&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{failedET}}, nil)

			s := &store.Stores{EventTypeStore: etMock}

			resp, err := MapEventSpecificationResponse(ctx, eventSpec, s)

			Expect(err).NotTo(HaveOccurred())
			// Parent is Complete/Done, but sub-resource is Failed → OverallStatus must be "failed"
			Expect(resp.State).To(Equal(api.Complete))
			Expect(resp.OverallStatus).To(Equal(api.OverallStatusFailed))
			Expect(resp.Errors).To(HaveLen(1))
		})
	})
})
