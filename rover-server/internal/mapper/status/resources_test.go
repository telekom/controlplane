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
	v1 "github.com/telekom/controlplane/rover/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

// MockObjectStore is a mock implementation of the ObjectStore interface.
type MockObjectStore[T SubResource] struct {
	mock.Mock
}

// List is the mock implementation of the List method.
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
			Context: "Application: rover-local-sub",
			Details: "Condition: Ready, Status: False, Message: Approval is either rejected or suspended",
			Message: "Approval is either rejected or suspended",
			Resource: api.ResourceRef{
				ApiVersion: "rover.cp.ei.telekom.de/v1",
				Kind:       "Rover",
				Name:       "rover-local-sub",
				Namespace:  "poc--eni--hyperion",
			},
		},
	}

	expectNoProblems = []api.Problem{}
)

var _ = Describe("GetAllProblemsInSubResource", func() {
	Context("when sub-resource has problems", func() {
		It("returns problems", func() {
			// given
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{apiSubscription}}, nil).Once()

			// when
			problems, err := GetAllProblemsInSubResource(ctx, rover, mockStore)

			// then
			Expect(err).NotTo(HaveOccurred())
			Expect(problems).To(Equal(expectedProblems))
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when there are no problems in sub-resource", func() {
		It("returns no problems", func() {
			// given
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			mockStore.On("List", ctx, mock.Anything).Return(
				&commonStore.ListResponse[*apiv1.ApiSubscription]{
					Items: []*apiv1.ApiSubscription{}}, nil).Once()

			// when
			problems, err := GetAllProblemsInSubResource(ctx, rover, mockStore)

			// then
			Expect(err).NotTo(HaveOccurred())
			Expect(problems).To(Equal(expectNoProblems))
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when there is an error retrieving sub-resources", func() {
		It("returns the error", func() {
			// given
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			expectedError := fmt.Errorf("error retrieving sub-resources")
			mockStore.On("List", ctx, mock.Anything).Return(
				(*commonStore.ListResponse[*apiv1.ApiSubscription])(nil), expectedError).Once()

			// when
			problems, err := GetAllProblemsInSubResource(ctx, rover, mockStore)

			// then
			Expect(err).To(HaveOccurred())
			Expect(problems).To(BeNil())
			Expect(err).To(Equal(expectedError))
			mockStore.AssertExpectations(GinkgoT())
		})
	})
})

var _ = Describe("GetNotReadyCondition", func() {
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
