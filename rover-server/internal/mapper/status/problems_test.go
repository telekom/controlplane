// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	commonStore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/telekom/controlplane/rover-server/internal/api"
	roverStore "github.com/telekom/controlplane/rover-server/pkg/store"
)

// MockObjectStore is a mock implementation of the ObjectStore interface.
type MockObjectStore[T SubResource] struct {
	mock.Mock
}

func (m *MockObjectStore[T]) List(ctx context.Context, opts commonStore.ListOpts) (*commonStore.ListResponse[T], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*commonStore.ListResponse[T]), args.Error(1)
}

func (m *MockObjectStore[T]) Info() (schema.GroupVersionResource, schema.GroupVersionKind) {
	args := m.Called()
	return args.Get(0).(schema.GroupVersionResource), args.Get(1).(schema.GroupVersionKind)
}

func (m *MockObjectStore[T]) Get(ctx context.Context, namespace string, name string) (T, error) {
	args := m.Called(ctx, namespace, name)
	return args.Get(0).(T), args.Error(1)
}

func (m *MockObjectStore[T]) CreateOrReplace(ctx context.Context, obj T) error {
	args := m.Called(ctx, obj)
	return args.Error(0)
}

func (m *MockObjectStore[T]) Patch(ctx context.Context, namespace, name string, ops ...commonStore.Patch) (T, error) {
	args := m.Called(ctx, namespace, name, ops)
	return args.Get(0).(T), args.Error(1)
}

func (m *MockObjectStore[T]) Delete(ctx context.Context, namespace string, name string) error {
	args := m.Called(ctx, namespace, name)
	return args.Error(0)
}

func (m *MockObjectStore[T]) Ready() bool {
	return true
}

var (
	rover = &v1.Rover{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rover.cp.ei.telekom.de/v1",
			Kind:       "Rover",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rover-local-sub",
			Namespace: "poc--eni--hyperion",
		},
		Status: v1.RoverStatus{
			Application: ptr.To(types.ObjectRef{
				Name:      "rover-local-sub",
				Namespace: "poc--eni--hyperion",
			}),
		},
	}

	apiSubscription = &apiv1.ApiSubscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "api.cp.ei.telekom.de/v1",
			Kind:       "ApiSubscription",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rover-local-sub--my-api-sub",
			Namespace: "poc--eni--hyperion",
		},
		Status: apiv1.ApiSubscriptionStatus{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "NoApproval",
					Message: "Approval is either rejected or suspended",
				},
			},
		},
	}

	expectedProblems = []api.Problem{
		{
			Cause:   "NoApproval",
			Details: "Condition: Ready, Status: False, Message: Approval is either rejected or suspended",
			Message: "Approval is either rejected or suspended",
			Resource: api.ResourceRef{
				ApiVersion: "api.cp.ei.telekom.de/v1",
				Kind:       "ApiSubscription",
				Name:       "rover-local-sub--my-api-sub",
				Namespace:  "poc--eni--hyperion",
			},
		},
	}

	expectNoProblems = []api.Problem{}
)

var _ = Describe("getAllProblemsInSubResource", func() {
	Context("when sub-resource has problems", func() {
		It("returns problems and worst overall status", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{apiSubscription}}, nil).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Problems).To(Equal(expectedProblems))
			// apiSubscription has Ready=False/NoApproval (not a processing-equivalent reason)
			// and no Processing condition → falls back to Ready=False → Blocked.
			Expect(result.WorstOverallStatus).To(Equal(api.OverallStatusBlocked))
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when sub-resource has a blocked status", func() {
		It("returns the blocked worst overall status", func() {
			ctx := context.Background()

			blockedSub := &apiv1.ApiSubscription{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "api.cp.ei.telekom.de/v1",
					Kind:       "ApiSubscription",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "blocked-sub",
					Namespace: "poc--eni--hyperion",
				},
				Status: apiv1.ApiSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:   condition.ConditionTypeProcessing,
							Status: metav1.ConditionFalse,
							Reason: "Done",
						},
						{
							Type:    condition.ConditionTypeReady,
							Status:  metav1.ConditionFalse,
							Reason:  "SubResourceNotReady",
							Message: "Sub-resource is blocked",
						},
					},
				},
			}

			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{blockedSub}}, nil).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Problems).To(HaveLen(1))
			Expect(result.WorstOverallStatus).To(Equal(api.OverallStatusProcessing))
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when a sub-resource handler returned a blocked error next to a ready sub-resource", func() {
		It("reports only the blocked sub-resource, as blocked, with its message", func() {
			ctx := context.Background()

			newSub := func(name string, conditions ...metav1.Condition) *apiv1.ApiSubscription {
				return &apiv1.ApiSubscription{
					TypeMeta:   metav1.TypeMeta{APIVersion: "api.cp.ei.telekom.de/v1", Kind: "ApiSubscription"},
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "poc--eni--hyperion"},
					Status:     apiv1.ApiSubscriptionStatus{Conditions: conditions},
				}
			}
			blockedSub := newSub(
				"blocked-sub",
				condition.NewBlockedCondition("consumer cannot be resolved"),
				condition.NewNotReadyCondition(condition.ReasonBlocked, "consumer cannot be resolved"),
			)
			readySub := newSub(
				"ready-sub",
				condition.NewDoneProcessingCondition("done"),
				condition.NewReadyCondition(condition.ReasonProvisioned, "ready"),
			)

			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{blockedSub, readySub},
				}, nil,
			).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorstOverallStatus).To(Equal(api.OverallStatusBlocked))
			Expect(result.Problems).To(HaveLen(1))
			Expect(result.Problems[0].Cause).To(Equal(condition.ReasonBlocked))
			Expect(result.Problems[0].Message).To(Equal("consumer cannot be resolved"))
			Expect(result.Problems[0].Resource.Kind).To(Equal("ApiSubscription"))
			Expect(result.Problems[0].Resource.Name).To(Equal("blocked-sub"))
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when there are no problems in sub-resource", func() {
		It("returns no problems and zero-value worst overall status", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{}}, nil).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Problems).To(Equal(expectNoProblems))
			Expect(result.WorstOverallStatus).To(Equal(api.OverallStatus("")))
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when there is an error retrieving sub-resources", func() {
		It("returns the error", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			expectedError := fmt.Errorf("error retrieving sub-resources")
			mockStore.On("List", ctx, mock.Anything).Return(
				(*commonStore.ListResponse[*apiv1.ApiSubscription])(nil), expectedError).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(ProblemsResult{}))
			Expect(err).To(Equal(expectedError))
			mockStore.AssertExpectations(GinkgoT())
		})
	})
})

var _ = Describe("getNotReadyCondition", func() {
	Context("when there is a not ready condition", func() {
		It("returns the not ready condition", func() {
			conditions := []metav1.Condition{
				{
					Type:    condition.ConditionTypeReady,
					Status:  metav1.ConditionFalse,
					Reason:  "SomeReason",
					Message: "Not ready",
				},
				{
					Type:   "OtherCondition",
					Status: metav1.ConditionTrue,
				},
			}

			result := getNotReadyCondition(conditions)

			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal(metav1.ConditionFalse))
			Expect(result.Reason).To(Equal("SomeReason"))
			Expect(result.Message).To(Equal("Not ready"))
		})
	})

	Context("when there is no not ready condition", func() {
		It("returns nil", func() {
			conditions := []metav1.Condition{
				{
					Type:   condition.ConditionTypeReady,
					Status: metav1.ConditionTrue,
				},
				{
					Type:   "OtherCondition",
					Status: metav1.ConditionTrue,
				},
			}

			result := getNotReadyCondition(conditions)

			Expect(result).To(BeNil())
		})
	})

	Context("when conditions are empty", func() {
		It("returns nil", func() {
			var conditions []metav1.Condition

			result := getNotReadyCondition(conditions)

			Expect(result).To(BeNil())
		})
	})
})

var _ = Describe("mapProblemsToStateInfos", func() {
	Context("when mapping problems to state infos", func() {
		It("returns correctly mapped state infos", func() {
			problems := []api.Problem{
				{Message: "Problem 1", Context: "Context 1", Cause: "Cause 1"},
				{Message: "Problem 2", Context: "Context 2", Cause: "Cause 2"},
			}
			expectedStateInfos := []api.StateInfo{
				{Message: "Problem 1", Cause: "Cause 1"},
				{Message: "Problem 2", Cause: "Cause 2"},
			}

			stateInfos := mapProblemsToStateInfos(problems)

			Expect(stateInfos).To(Equal(expectedStateInfos))
		})
	})

	Context("when given empty problems slice", func() {
		It("returns empty state infos", func() {
			problems := []api.Problem{}
			expectedStateInfos := []api.StateInfo{}

			stateInfos := mapProblemsToStateInfos(problems)

			Expect(stateInfos).To(Equal(expectedStateInfos))
		})
	})

	Context("when given nil problems", func() {
		It("returns empty state infos", func() {
			var problems []api.Problem
			expectedStateInfos := []api.StateInfo{}

			stateInfos := mapProblemsToStateInfos(problems)

			Expect(stateInfos).To(Equal(expectedStateInfos))
		})
	})
})

var _ = Describe("hasRefs", func() {
	It("returns false for nil slice", func() {
		Expect(hasRefs(nil)).To(BeFalse())
	})
	It("returns false for empty slice", func() {
		Expect(hasRefs([]types.ObjectRef{})).To(BeFalse())
	})
	It("returns true for non-empty slice", func() {
		Expect(hasRefs([]types.ObjectRef{{Name: "a", Namespace: "ns"}})).To(BeTrue())
	})
})

var _ = Describe("getAllProblemsInSubResource HasStale", func() {
	Context("when a sub-resource has stale conditions", func() {
		It("returns HasStale true", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			staleAPISubscription := &apiv1.ApiSubscription{
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
					Items: []*apiv1.ApiSubscription{staleAPISubscription}}, nil).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.HasStale).To(BeTrue())
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when all sub-resources are current", func() {
		It("returns HasStale false", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			currentAPISubscription := &apiv1.ApiSubscription{
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
					Items: []*apiv1.ApiSubscription{currentAPISubscription}}, nil).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.HasStale).To(BeFalse())
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when sub-resources list is empty", func() {
		It("returns HasStale false", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{}}, nil).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.HasStale).To(BeFalse())
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when sub-resource has ObservedGeneration zero (backward compat)", func() {
		It("returns HasStale false", func() {
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])

			legacyAPISubscription := &apiv1.ApiSubscription{
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
					Items: []*apiv1.ApiSubscription{legacyAPISubscription}}, nil).Once()

			result, err := getAllProblemsInSubResource(ctx, rover, mockStore)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.HasStale).To(BeFalse())
			mockStore.AssertExpectations(GinkgoT())
		})
	})
})

var _ = Describe("GetAllRoverProblems agentic sub-resources", func() {
	const namespace = "poc--eni--hyperion"

	readyConds := func() []metav1.Condition {
		return []metav1.Condition{
			{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Done", ObservedGeneration: 1},
			{Type: condition.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: 1},
		}
	}
	blockedConds := []metav1.Condition{
		{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionFalse, Reason: "Blocked", ObservedGeneration: 1},
		{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "NoApproval", Message: "not approved", ObservedGeneration: 1},
	}
	processingConds := []metav1.Condition{
		{Type: condition.ConditionTypeProcessing, Status: metav1.ConditionTrue, Reason: "Processing", ObservedGeneration: 1},
		{Type: condition.ConditionTypeReady, Status: metav1.ConditionFalse, Reason: "Processing", Message: "in progress", ObservedGeneration: 1},
	}

	newRover := func(exposures, subscriptions []types.ObjectRef) *v1.Rover {
		return &v1.Rover{
			ObjectMeta: metav1.ObjectMeta{Name: "rover", Namespace: namespace, UID: "rover-uid"},
			Status: v1.RoverStatus{
				AgenticExposures:     exposures,
				AgenticSubscriptions: subscriptions,
			},
		}
	}
	refs := []types.ObjectRef{{Name: "child", Namespace: namespace}}

	newExposure := func(gen int64, conds []metav1.Condition) *agenticv1.AgenticExposure {
		return &agenticv1.AgenticExposure{
			TypeMeta:   metav1.TypeMeta{APIVersion: agenticv1.GroupVersion.String(), Kind: "AgenticExposure"},
			ObjectMeta: metav1.ObjectMeta{Name: "exposure", Namespace: namespace, Generation: gen},
			Status:     agenticv1.AgenticExposureStatus{Conditions: conds},
		}
	}
	newSubscription := func(gen int64, conds []metav1.Condition) *agenticv1.AgenticSubscription {
		return &agenticv1.AgenticSubscription{
			TypeMeta:   metav1.TypeMeta{APIVersion: agenticv1.GroupVersion.String(), Kind: "AgenticSubscription"},
			ObjectMeta: metav1.ObjectMeta{Name: "subscription", Namespace: namespace, Generation: gen},
			Status:     agenticv1.AgenticSubscriptionStatus{Conditions: conds},
		}
	}

	type expectation struct {
		worst        api.OverallStatus
		problemKinds []string
		stale        bool
		active       bool
	}

	run := func(exposure *agenticv1.AgenticExposure, subscription *agenticv1.AgenticSubscription, exp expectation) {
		ctx := context.Background()
		expStore := new(MockObjectStore[*agenticv1.AgenticExposure])
		subStore := new(MockObjectStore[*agenticv1.AgenticSubscription])
		ownerFilter := mock.MatchedBy(func(opts commonStore.ListOpts) bool {
			return opts.Prefix == namespace && len(opts.Filters) == 1 && opts.Filters[0].Value == "rover-uid"
		})

		var exposureRefs, subscriptionRefs []types.ObjectRef
		if exposure != nil {
			exposureRefs = refs
			expStore.On("List", ctx, ownerFilter).Return(
				&commonStore.ListResponse[*agenticv1.AgenticExposure]{Items: []*agenticv1.AgenticExposure{exposure}}, nil).Once()
		}
		if subscription != nil {
			subscriptionRefs = refs
			subStore.On("List", ctx, ownerFilter).Return(
				&commonStore.ListResponse[*agenticv1.AgenticSubscription]{Items: []*agenticv1.AgenticSubscription{subscription}}, nil).Once()
		}
		s := &roverStore.Stores{AgenticExposureStore: expStore, AgenticSubscriptionStore: subStore}

		result, err := GetAllRoverProblems(ctx, newRover(exposureRefs, subscriptionRefs), s)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.WorstOverallStatus).To(Equal(exp.worst))
		Expect(result.HasStale).To(Equal(exp.stale))
		Expect(result.HasActiveSubResources).To(Equal(exp.active))
		kinds := make([]string, 0, len(result.Problems))
		for _, p := range result.Problems {
			kinds = append(kinds, p.Resource.Kind)
		}
		Expect(kinds).To(ConsistOf(exp.problemKinds))
		expStore.AssertExpectations(GinkgoT())
		subStore.AssertExpectations(GinkgoT())
	}

	DescribeTable("aggregates agentic sub-resource status",
		run,
		Entry("exposure blocked", newExposure(1, blockedConds), nil,
			expectation{worst: api.OverallStatusBlocked, problemKinds: []string{"AgenticExposure"}}),
		Entry("subscription blocked", nil, newSubscription(1, blockedConds),
			expectation{worst: api.OverallStatusBlocked, problemKinds: []string{"AgenticSubscription"}}),
		Entry("both ready", newExposure(1, readyConds()), newSubscription(1, readyConds()),
			expectation{worst: api.OverallStatusComplete, problemKinds: []string{}}),
		Entry("exposure stale", newExposure(2, readyConds()), nil,
			expectation{worst: api.OverallStatusComplete, problemKinds: []string{}, stale: true}),
		Entry("subscription stale", nil, newSubscription(2, readyConds()),
			expectation{worst: api.OverallStatusComplete, problemKinds: []string{}, stale: true}),
		Entry("exposure processing", newExposure(1, processingConds), nil,
			expectation{worst: api.OverallStatusProcessing, problemKinds: []string{"AgenticExposure"}, active: true}),
		Entry("subscription processing, exposure blocked", newExposure(1, blockedConds), newSubscription(1, processingConds),
			expectation{worst: api.OverallStatusProcessing, problemKinds: []string{"AgenticExposure", "AgenticSubscription"}, active: true}),
	)

	It("does not query agentic stores when refs are empty", func() {
		expStore := new(MockObjectStore[*agenticv1.AgenticExposure])
		subStore := new(MockObjectStore[*agenticv1.AgenticSubscription])
		s := &roverStore.Stores{AgenticExposureStore: expStore, AgenticSubscriptionStore: subStore}

		result, err := GetAllRoverProblems(context.Background(), newRover(nil, []types.ObjectRef{}), s)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ProblemsResult{}))
		expStore.AssertNotCalled(GinkgoT(), "List", mock.Anything, mock.Anything)
		subStore.AssertNotCalled(GinkgoT(), "List", mock.Anything, mock.Anything)
	})

	DescribeTable("propagates store errors",
		func(exposureFails bool) {
			ctx := context.Background()
			storeErr := errors.New("store unavailable")
			expStore := new(MockObjectStore[*agenticv1.AgenticExposure])
			subStore := new(MockObjectStore[*agenticv1.AgenticSubscription])
			var rov *v1.Rover
			if exposureFails {
				expStore.On("List", ctx, mock.Anything).Return((*commonStore.ListResponse[*agenticv1.AgenticExposure])(nil), storeErr).Once()
				rov = newRover(refs, nil)
			} else {
				subStore.On("List", ctx, mock.Anything).Return((*commonStore.ListResponse[*agenticv1.AgenticSubscription])(nil), storeErr).Once()
				rov = newRover(nil, refs)
			}
			s := &roverStore.Stores{AgenticExposureStore: expStore, AgenticSubscriptionStore: subStore}

			result, err := GetAllRoverProblems(ctx, rov, s)

			Expect(err).To(MatchError(storeErr))
			Expect(result).To(Equal(ProblemsResult{}))
			expStore.AssertExpectations(GinkgoT())
			subStore.AssertExpectations(GinkgoT())
		},
		Entry("AgenticExposure store", true),
		Entry("AgenticSubscription store", false),
	)
})
