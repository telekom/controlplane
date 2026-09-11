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
				Expect(result).To(Equal(status))

				// Verify the mock was called as expected
				handler.AssertExpectations(GinkgoT())
			})
		})

		// This test is removed as it causes flaky behavior in CI
		// We can test context timeout handling without mocking

		Context("when the context times out", func() {
			It("should return the last known status alongside the error", func() {
				// Use a very short timeout so it fires quickly
				shortTimeout := 250 * time.Millisecond
				shortPoller := common.NewStatusPoller(handler, func(_ context.Context, _ types.ObjectStatus) (bool, error) {
					return true, nil // always continue polling
				}, shortTimeout, 50*time.Millisecond)

				// The status that will be returned on each poll
				processingStatus := &common.ObjectStatusResponse{
					ProcessingState: "processing",
					OverallStatus:   "processing",
				}

				handler.EXPECT().Status(mock.Anything, "test-resource").Return(processingStatus, nil).Maybe()

				// Start polling - should timeout
				result, err := shortPoller.Start(testCtx, "test-resource")

				// Verify that timeout error is returned with the last status
				Expect(err).To(MatchError(context.DeadlineExceeded))
				Expect(result).To(Equal(processingStatus))
			})

			It("should return nil status if no poll completed before timeout", func() {
				// Use a timeout shorter than the poll interval
				shortTimeout := 10 * time.Millisecond
				shortPoller := common.NewStatusPoller(handler, nil, shortTimeout, 1*time.Second)

				// Start polling - should timeout before any poll happens
				result, err := shortPoller.Start(testCtx, "test-resource")

				// Verify that timeout error is returned with nil status
				Expect(err).To(MatchError(context.DeadlineExceeded))
				Expect(result).To(BeNil())
			})
		})

		Context("when no evaluation function is provided", func() {
			It("should use the default eval function", func() {
				// Create a poller with nil eval function (will use default)
				defaultPoller := common.NewStatusPoller(handler, nil, timeout, interval)

				// First call - status not done yet
				inProgressStatus := &common.ObjectStatusResponse{
					ProcessingState: "processing",
					OverallStatus:   "processing",
				}

				// Second call - status is complete
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

			It("should return an error when overall status is failed", func() {
				// Create a poller with nil eval function (will use default)
				defaultPoller := common.NewStatusPoller(handler, nil, timeout, interval)

				// Status transitions: processing -> failed
				inProgressStatus := &common.ObjectStatusResponse{
					ProcessingState: "processing",
					OverallStatus:   "processing",
				}
				failedStatus := &common.ObjectStatusResponse{
					ProcessingState: "failed",
					OverallStatus:   "failed",
				}

				// Configure the mock to return statuses
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(inProgressStatus, nil).Once()
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(failedStatus, nil).Once()

				// Start polling
				result, err := defaultPoller.Start(testCtx, "test-resource")

				// Verify that an error is returned for failed state
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("resource processing failed"))
				Expect(result).To(Equal(failedStatus))

				// Verify the mock was called as expected
				handler.AssertExpectations(GinkgoT())
			})

			It("should stop polling when overall status is blocked", func() {
				// Create a poller with nil eval function (will use default)
				defaultPoller := common.NewStatusPoller(handler, nil, timeout, interval)

				// Status transitions: processing -> blocked
				inProgressStatus := &common.ObjectStatusResponse{
					ProcessingState: "processing",
					OverallStatus:   "processing",
				}
				blockedStatus := &common.ObjectStatusResponse{
					ProcessingState: "done",
					OverallStatus:   "blocked",
				}

				// Configure the mock to return statuses
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(inProgressStatus, nil).Once()
				handler.EXPECT().Status(mock.Anything, "test-resource").Return(blockedStatus, nil).Once()

				// Start polling
				result, err := defaultPoller.Start(testCtx, "test-resource")

				// Verify that blocked stops polling without error
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(blockedStatus))

				// Verify the mock was called as expected
				handler.AssertExpectations(GinkgoT())
			})
		})
	})
})
