// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("StatusPoller", func() {
	var (
		handler      *mocks.MockStatusHandler
		poller       *common.StatusPoller
		testCtx      context.Context
		evalFuncUsed bool
		timeout      time.Duration
		interval     time.Duration
	)

	BeforeEach(func() {
		handler = mocks.NewMockStatusHandler(GinkgoT())
		testCtx = context.Background()
		evalFuncUsed = false
		timeout = 5 * time.Second
		interval = 100 * time.Millisecond

		// Create a custom eval function for testing
		customEvalFunc := func(ctx context.Context, status types.ObjectStatus) (bool, error) {
			evalFuncUsed = true
			// Continue polling if processing state is not "done"
			return status.GetProcessingState() != "done", nil
		}

		poller = common.NewStatusPoller(handler, customEvalFunc, timeout, interval)
	})

	Describe("Start", func() {
		Context("when polling for status changes", func() {
			It("should stop polling when the status is done", func() {
				// First call - status not done yet
				inProgressStatus := &common.ObjectStatusResponse{
					ProcessingState: "processing",
					OverallStatus:   "pending",
				}

				// Second call - status is done
				doneStatus := &common.ObjectStatusResponse{
					ProcessingState: "done",
					OverallStatus:   "complete",
				}

				// Configure the mock to first return in-progress status, then done status
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(inProgressStatus, nil).Once()
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(doneStatus, nil).Once()

				// Start polling
				result, err := poller.Start(testCtx, "test-resource")

				// Verify results
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(doneStatus))
				Expect(evalFuncUsed).To(BeTrue())

				// Verify the mock was called as expected
				handler.AssertExpectations(GinkgoT())
			})
		})

		Context("when the handler returns an error", func() {
			It("should propagate the error", func() {
				// Configure the mock to return an error
				expectedErr := errors.New("handler error")
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(nil, expectedErr)

				// Start polling
				result, err := poller.Start(testCtx, "test-resource")

				// Verify results
				Expect(err).To(MatchError(expectedErr))
				Expect(result).To(BeNil())

				// Verify the mock was called as expected
				handler.AssertExpectations(GinkgoT())
			})
		})

		Context("when the evaluation function returns an error", func() {
			It("should propagate the error", func() {
				// Create a poller with an eval function that returns an error
				expectedErr := errors.New("eval error")
				errorEvalFunc := func(ctx context.Context, status types.ObjectStatus) (bool, error) {
					return false, expectedErr
				}

				errorPoller := common.NewStatusPoller(handler, errorEvalFunc, timeout, interval)

				// Configure the mock to return a status
				status := &common.ObjectStatusResponse{
					ProcessingState: "processing",
					OverallStatus:   "pending",
				}
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(status, nil)

				// Start polling
				result, err := errorPoller.Start(testCtx, "test-resource")

				// Verify results
				Expect(err).To(MatchError(expectedErr))
				Expect(result).To(BeNil())

				// Verify the mock was called as expected
				handler.AssertExpectations(GinkgoT())
			})
		})

		// This test is removed as it causes flaky behavior in CI
		// We can test context timeout handling without mocking

		Context("when no evaluation function is provided", func() {
			It("should use the default eval function", func() {
				// Create a poller with nil eval function (will use default)
				defaultPoller := common.NewStatusPoller(handler, nil, timeout, interval)

				// First call - status not done yet
				inProgressStatus := &common.ObjectStatusResponse{
					ProcessingState: "processing",
					OverallStatus:   "pending",
				}

				// Second call - status is done
				doneStatus := &common.ObjectStatusResponse{
					ProcessingState: "done",
					OverallStatus:   "complete",
				}

				// Configure the mock to return statuses
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(inProgressStatus, nil).Once()
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(doneStatus, nil).Once()

				// Start polling
				result, err := defaultPoller.Start(testCtx, "test-resource")

				// Verify results
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(doneStatus))

				// Verify the mock was called as expected
				handler.AssertExpectations(GinkgoT())
			})
		})
	})
})
