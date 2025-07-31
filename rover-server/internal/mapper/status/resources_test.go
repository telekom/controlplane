// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestGetAllProblemsInSubResource_ReturnsProblems(t *testing.T) {
	// given
	ctx := context.Background()
	mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
	mockStore.On("List", ctx, mock.Anything).Return(
		&commonStore.ListResponse[*apiv1.ApiSubscription]{
			Items: []*apiv1.ApiSubscription{apiSubscription}}, nil).Once()

	// when
	problems, err := GetAllProblemsInSubResource[*apiv1.ApiSubscription](ctx, rover, mockStore)

	// then
	assert.NoError(t, err)
	assert.Equal(t, expectedProblems, problems)
	mockStore.AssertExpectations(t)
}

func TestGetAllProblemsInSubResource_NoProblems(t *testing.T) {
	// given
	ctx := context.Background()
	mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
	mockStore.On("List", ctx, mock.Anything).Return(
		&commonStore.ListResponse[*apiv1.ApiSubscription]{
			Items: []*apiv1.ApiSubscription{}}, nil).Once()

	// when
	problems, err := GetAllProblemsInSubResource[*apiv1.ApiSubscription](ctx, rover, mockStore)

	// then
	assert.NoError(t, err)
	assert.Equal(t, expectNoProblems, problems)
	mockStore.AssertExpectations(t)
}

func TestGetAllProblemsInSubResource_Error(t *testing.T) {
	// given
	ctx := context.Background()
	mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
	expectedError := fmt.Errorf("error retrieving sub-resources")
	mockStore.On("List", ctx, mock.Anything).Return(
		(*commonStore.ListResponse[*apiv1.ApiSubscription])(nil), expectedError).Once()

	// when
	problems, err := GetAllProblemsInSubResource[*apiv1.ApiSubscription](ctx, rover, mockStore)

	// then
	assert.Error(t, err)
	assert.Nil(t, problems)
	assert.Equal(t, expectedError, err)
	mockStore.AssertExpectations(t)
}

func TestGetNotReadyCondition_ReturnsNotReadyCondition(t *testing.T) {
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

	assert.NotNil(t, result)
	assert.Equal(t, metav1.ConditionFalse, result.Status)
	assert.Equal(t, "SomeReason", result.Reason)
	assert.Equal(t, "Not ready", result.Message)
}

func TestGetNotReadyCondition_NoNotReadyCondition(t *testing.T) {
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

	assert.Nil(t, result)
}

func TestGetNotReadyCondition_EmptyConditions(t *testing.T) {
	var conditions []metav1.Condition

	result := getNotReadyCondition(conditions)

	assert.Nil(t, result)
}
