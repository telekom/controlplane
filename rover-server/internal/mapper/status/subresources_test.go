// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	v1 "github.com/telekom/controlplane/rover/api/v1"
)

var _ = Describe("GetAllSubResources", func() {
	Context("when rover is nil", func() {
		It("returns an error", func() {
			// given
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			var rover *v1.Rover = nil

			// when
			result, err := getAllSubResources(ctx, rover, mockStore)

			// then
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("rover resource is not processed and does not contain an application"))
			// Note: mock assertions are still using testify style since we need the mock object
			mockStore.AssertExpectations(GinkgoT())
		})
	})

	Context("when rover application is nil", func() {
		It("returns an error", func() {
			// given
			ctx := context.Background()
			mockStore := new(MockObjectStore[*apiv1.ApiSubscription])
			rover := &v1.Rover{
				Status: v1.RoverStatus{
					// Application is nil
				},
			}

			// when
			result, err := getAllSubResources(ctx, rover, mockStore)

			// then
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("rover resource is not processed and does not contain an application"))
			// Note: mock assertions are still using testify style since we need the mock object
			mockStore.AssertExpectations(GinkgoT())
		})
	})
})
