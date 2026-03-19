// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	roverStore "github.com/telekom/controlplane/rover-server/pkg/store"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("anySubResourceStale", func() {
	Context("when a sub-resource has stale conditions", func() {
		It("returns true", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			staleApiSubscription := &apiv1.ApiSubscription{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "api.cp.ei.telekom.de/v1",
					Kind:       "ApiSubscription",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "stale-sub",
					Namespace:  "poc--eni--hyperion",
					Generation: 5,
				},
				Status: apiv1.ApiSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:               condition.ConditionTypeProcessing,
							Status:             metav1.ConditionFalse,
							Reason:             "Done",
							ObservedGeneration: 3,
						},
						{
							Type:               condition.ConditionTypeReady,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 3,
						},
					},
				},
			}

			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{staleApiSubscription}}, nil).Once()

			stale, err := anySubResourceStale(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(stale).To(BeTrue())
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when all sub-resources are current", func() {
		It("returns false", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			currentApiSubscription := &apiv1.ApiSubscription{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "api.cp.ei.telekom.de/v1",
					Kind:       "ApiSubscription",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "current-sub",
					Namespace:  "poc--eni--hyperion",
					Generation: 3,
				},
				Status: apiv1.ApiSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:               condition.ConditionTypeProcessing,
							Status:             metav1.ConditionFalse,
							Reason:             "Done",
							ObservedGeneration: 3,
						},
						{
							Type:               condition.ConditionTypeReady,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 3,
						},
					},
				},
			}

			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{currentApiSubscription}}, nil).Once()

			stale, err := anySubResourceStale(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(stale).To(BeFalse())
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when sub-resources list is empty", func() {
		It("returns false", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{}}, nil).Once()

			stale, err := anySubResourceStale(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(stale).To(BeFalse())
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when the store returns an error", func() {
		It("returns the error", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			expectedError := fmt.Errorf("store error")

			mockStore.On("List", ctx, mock.Anything).Return(
				(*commonStore.ListResponse[*apiv1.ApiSubscription])(nil), expectedError).Once()

			stale, err := anySubResourceStale(ctx, rover, mockStore)

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(expectedError))
			Expect(stale).To(BeFalse())
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when sub-resource has ObservedGeneration zero (backward compat)", func() {
		It("returns false", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			legacyApiSubscription := &apiv1.ApiSubscription{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "api.cp.ei.telekom.de/v1",
					Kind:       "ApiSubscription",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "legacy-sub",
					Namespace:  "poc--eni--hyperion",
					Generation: 5,
				},
				Status: apiv1.ApiSubscriptionStatus{
					Conditions: []metav1.Condition{
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
					},
				},
			}

			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{legacyApiSubscription}}, nil).Once()

			stale, err := anySubResourceStale(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(stale).To(BeFalse())
			mockStore.AssertExpectations(GinkgoT())
		})
	})
})

// --- Public function tests ---

var _ = Describe("AnyRoverSubResourceStale", func() {
	It("returns false when rover has no sub-resource refs", func() {
		emptyRover := &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "ns", UID: "uid-1"},
			Status:     v1.RoverStatus{},
		}
		s := &roverStore.Stores{}

		stale, err := AnyRoverSubResourceStale(ctx, emptyRover, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeFalse())
	})

	It("returns true when ApiSubscription sub-resource is stale", func() {
		r := &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "ns", UID: "uid-2"},
			Status: v1.RoverStatus{
				ApiSubscriptions: []types.ObjectRef{{Name: "sub-1", Namespace: "ns"}},
			},
		}

		apiSubMock := new(MockObjectStore[*apiv1.ApiSubscription])
		staleSub := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "sub-1", Namespace: "ns", Generation: 5},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
				},
			},
		}
		apiSubMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.ApiSubscription]{Items: []*apiv1.ApiSubscription{staleSub}}, nil)

		s := &roverStore.Stores{APISubscriptionStore: apiSubMock}

		stale, err := AnyRoverSubResourceStale(ctx, r, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeTrue())
	})

	It("returns false when all sub-resources are current", func() {
		rWithExposure := &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "r2b", Namespace: "ns", UID: "uid-3b"},
			Status: v1.RoverStatus{
				ApiExposures: []types.ObjectRef{{Name: "exp-1", Namespace: "ns"}},
			},
		}

		expMock := new(MockObjectStore[*apiv1.ApiExposure])
		currentExp := &apiv1.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "exp-1", Namespace: "ns", Generation: 3},
			Status: apiv1.ApiExposureStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
				},
			},
		}
		expMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.ApiExposure]{Items: []*apiv1.ApiExposure{currentExp}}, nil)

		s := &roverStore.Stores{APIExposureStore: expMock}

		stale, err := AnyRoverSubResourceStale(ctx, rWithExposure, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeFalse())
	})

	It("returns error when a store query fails", func() {
		r := &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "r3", Namespace: "ns", UID: "uid-4"},
			Status: v1.RoverStatus{
				ApiSubscriptions: []types.ObjectRef{{Name: "sub-1", Namespace: "ns"}},
			},
		}

		apiSubMock := new(MockObjectStore[*apiv1.ApiSubscription])
		apiSubMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*apiv1.ApiSubscription])(nil), fmt.Errorf("connection refused"))

		s := &roverStore.Stores{APISubscriptionStore: apiSubMock}

		stale, err := AnyRoverSubResourceStale(ctx, r, s)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
		Expect(stale).To(BeFalse())
	})

	It("checks multiple sub-resource types and returns true when second is stale", func() {
		r := &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "r4", Namespace: "ns", UID: "uid-5"},
			Status: v1.RoverStatus{
				ApiSubscriptions: []types.ObjectRef{{Name: "sub-1", Namespace: "ns"}},
				ApiExposures:     []types.ObjectRef{{Name: "exp-1", Namespace: "ns"}},
			},
		}

		// ApiSubscription: current
		apiSubMock := new(MockObjectStore[*apiv1.ApiSubscription])
		currentSub := &apiv1.ApiSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "sub-1", Namespace: "ns", Generation: 3},
			Status: apiv1.ApiSubscriptionStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
				},
			},
		}
		apiSubMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.ApiSubscription]{Items: []*apiv1.ApiSubscription{currentSub}}, nil)

		// ApiExposure: stale
		expMock := new(MockObjectStore[*apiv1.ApiExposure])
		staleExp := &apiv1.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{Name: "exp-1", Namespace: "ns", Generation: 5},
			Status: apiv1.ApiExposureStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 2},
				},
			},
		}
		expMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.ApiExposure]{Items: []*apiv1.ApiExposure{staleExp}}, nil)

		s := &roverStore.Stores{
			APISubscriptionStore: apiSubMock,
			APIExposureStore:     expMock,
		}

		stale, err := AnyRoverSubResourceStale(ctx, r, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeTrue())
	})
})

var _ = Describe("AnyAPISpecificationSubResourceStale", func() {
	It("returns false when Api ref is empty (short-circuit)", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-api", Namespace: "ns", UID: "uid-a1"},
			Status:     v1.ApiSpecificationStatus{},
		}
		s := &roverStore.Stores{}

		stale, err := AnyAPISpecificationSubResourceStale(ctx, apiSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeFalse())
	})

	It("returns true when Api sub-resource is stale", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-1", Namespace: "ns", UID: "uid-a2", Generation: 2},
			Status: v1.ApiSpecificationStatus{
				Api: types.ObjectRef{Name: "api-1", Namespace: "ns"},
			},
		}

		apiMock := new(MockObjectStore[*apiv1.Api])
		staleAPI := &apiv1.Api{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "ns", Generation: 5},
			Status: apiv1.ApiStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
				},
			},
		}
		apiMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{staleAPI}}, nil)

		s := &roverStore.Stores{APIStore: apiMock}

		stale, err := AnyAPISpecificationSubResourceStale(ctx, apiSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeTrue())
	})

	It("returns false when Api sub-resource is current", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-2", Namespace: "ns", UID: "uid-a3", Generation: 2},
			Status: v1.ApiSpecificationStatus{
				Api: types.ObjectRef{Name: "api-1", Namespace: "ns"},
			},
		}

		apiMock := new(MockObjectStore[*apiv1.Api])
		currentAPI := &apiv1.Api{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "ns", Generation: 3},
			Status: apiv1.ApiStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
				},
			},
		}
		apiMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*apiv1.Api]{Items: []*apiv1.Api{currentAPI}}, nil)

		s := &roverStore.Stores{APIStore: apiMock}

		stale, err := AnyAPISpecificationSubResourceStale(ctx, apiSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeFalse())
	})

	It("returns error when store query fails", func() {
		apiSpec := &v1.ApiSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "spec-3", Namespace: "ns", UID: "uid-a4"},
			Status: v1.ApiSpecificationStatus{
				Api: types.ObjectRef{Name: "api-1", Namespace: "ns"},
			},
		}

		apiMock := new(MockObjectStore[*apiv1.Api])
		apiMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*apiv1.Api])(nil), fmt.Errorf("api store error"))

		s := &roverStore.Stores{APIStore: apiMock}

		stale, err := AnyAPISpecificationSubResourceStale(ctx, apiSpec, s)

		Expect(err).To(HaveOccurred())
		Expect(stale).To(BeFalse())
	})
})

var _ = Describe("AnyEventSpecificationSubResourceStale", func() {
	It("returns false when EventType ref is empty (short-circuit)", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-event", Namespace: "ns", UID: "uid-e1"},
			Status:     v1.EventSpecificationStatus{},
		}
		s := &roverStore.Stores{}

		stale, err := AnyEventSpecificationSubResourceStale(ctx, eventSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeFalse())
	})

	It("returns true when EventType sub-resource is stale", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "ns", UID: "uid-e2", Generation: 2},
			Status: v1.EventSpecificationStatus{
				EventType: types.ObjectRef{Name: "et-1", Namespace: "ns"},
			},
		}

		etMock := new(MockObjectStore[*eventv1.EventType])
		staleET := &eventv1.EventType{
			ObjectMeta: metav1.ObjectMeta{Name: "et-1", Namespace: "ns", Generation: 5},
			Status: eventv1.EventTypeStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
				},
			},
		}
		etMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{staleET}}, nil)

		s := &roverStore.Stores{EventTypeStore: etMock}

		stale, err := AnyEventSpecificationSubResourceStale(ctx, eventSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeTrue())
	})

	It("returns false when EventType sub-resource is current", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-2", Namespace: "ns", UID: "uid-e3", Generation: 2},
			Status: v1.EventSpecificationStatus{
				EventType: types.ObjectRef{Name: "et-1", Namespace: "ns"},
			},
		}

		etMock := new(MockObjectStore[*eventv1.EventType])
		currentET := &eventv1.EventType{
			ObjectMeta: metav1.ObjectMeta{Name: "et-1", Namespace: "ns", Generation: 3},
			Status: eventv1.EventTypeStatus{
				Conditions: []metav1.Condition{
					{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 3},
				},
			},
		}
		etMock.On("List", mock.Anything, mock.Anything).Return(
			&commonStore.ListResponse[*eventv1.EventType]{Items: []*eventv1.EventType{currentET}}, nil)

		s := &roverStore.Stores{EventTypeStore: etMock}

		stale, err := AnyEventSpecificationSubResourceStale(ctx, eventSpec, s)

		Expect(err).NotTo(HaveOccurred())
		Expect(stale).To(BeFalse())
	})

	It("returns error when store query fails", func() {
		eventSpec := &v1.EventSpecification{
			ObjectMeta: metav1.ObjectMeta{Name: "es-3", Namespace: "ns", UID: "uid-e4"},
			Status: v1.EventSpecificationStatus{
				EventType: types.ObjectRef{Name: "et-1", Namespace: "ns"},
			},
		}

		etMock := new(MockObjectStore[*eventv1.EventType])
		etMock.On("List", mock.Anything, mock.Anything).Return(
			(*commonStore.ListResponse[*eventv1.EventType])(nil), fmt.Errorf("event type store error"))

		s := &roverStore.Stores{EventTypeStore: etMock}

		stale, err := AnyEventSpecificationSubResourceStale(ctx, eventSpec, s)

		Expect(err).To(HaveOccurred())
		Expect(stale).To(BeFalse())
	})
})
