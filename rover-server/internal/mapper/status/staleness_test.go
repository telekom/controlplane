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
