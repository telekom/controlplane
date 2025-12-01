// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package ctrlerrors_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	"github.com/telekom/controlplane/common/pkg/test"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MyBlockedError struct {
	msg string
}

func (e *MyBlockedError) Error() string {
	return e.msg
}

func (e *MyBlockedError) IsBlocked() bool {
	return true
}

type MyRetryableError struct {
	msg string
}

func (e *MyRetryableError) Error() string {
	return e.msg
}

func (e *MyRetryableError) IsRetryable() bool {
	return true
}

type MyRetryableWithDelayError struct {
	msg   string
	delay time.Duration
}

func (e *MyRetryableWithDelayError) Error() string {
	return e.msg
}

func (e *MyRetryableWithDelayError) IsRetryable() bool {
	return true
}

func (e *MyRetryableWithDelayError) RetryDelay() time.Duration {
	return e.delay
}

func TestCtrlerrors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ctrlerrors Suite")
}

var _ = Describe("Test Suite", func() {

	var recorder *mock.EventRecorder
	var ctx context.Context
	BeforeEach(func() {
		recorder = &mock.EventRecorder{}
		ctx = context.Background()
	})

	Context("BlockedError", func() {
		It("should correctly handle a blocked error", func() {
			ctrlErr := ctrlerrors.BlockedErrorf("This is a blocked error")
			Expect(ctrlErr.IsBlocked()).To(BeTrue())
			Expect(ctrlErr.Error()).To(Equal("This is a blocked error"))

			obj := test.NewObject("blocked-obj", "default")
			updated, result := ctrlerrors.HandleError(ctx, obj, ctrlErr, recorder)
			Expect(updated).To(BeTrue())
			Expect(result.RequeueAfter).To(BeNumerically(">", 30*time.Minute))
			condition := obj.GetConditions()[0]
			Expect(condition.Type).To(Equal("Processing"))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Blocked"))
			Expect(condition.Message).To(Equal("This is a blocked error"))
		})

		It("should not treat a non-blocked error as blocked", func() {
			ctrlErr := ctrlerrors.RetryableErrorf("This is a retryable error")
			Expect(ctrlErr.IsBlocked()).To(BeFalse())
		})

		It("should support custom blocked errors", func() {

			myErr := &MyBlockedError{msg: "Custom blocked error"}
			Expect(myErr.IsBlocked()).To(BeTrue())
			Expect(myErr.Error()).To(Equal("Custom blocked error"))

			obj := test.NewObject("custom-blocked-obj", "default")
			_, result := ctrlerrors.HandleError(ctx, obj, myErr, recorder)
			Expect(result.RequeueAfter).To(BeNumerically(">", 30*time.Minute))
			condition := obj.GetConditions()[0]
			Expect(condition.Type).To(Equal("Processing"))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Blocked"))
			Expect(condition.Message).To(Equal("Custom blocked error"))
		})
	})

	Context("RetryableError", func() {
		It("should correctly handle a retryable error", func() {
			ctrlErr := ctrlerrors.RetryableErrorf("This is a retryable error")
			Expect(ctrlErr.IsRetryable()).To(BeTrue())
			Expect(ctrlErr.Error()).To(Equal("This is a retryable error"))

			obj := test.NewObject("retryable-obj", "default")
			_, result := ctrlerrors.HandleError(ctx, obj, ctrlErr, recorder)
			Expect(result.RequeueAfter).NotTo(Equal(time.Duration(0)))
		})

		It("should support custom retryable errors", func() {
			myErr := &MyRetryableError{msg: "Custom retryable error"}
			Expect(myErr.IsRetryable()).To(BeTrue())
			Expect(myErr.Error()).To(Equal("Custom retryable error"))

			obj := test.NewObject("custom-retryable-obj", "default")
			_, result := ctrlerrors.HandleError(ctx, obj, myErr, recorder)
			Expect(result.RequeueAfter).NotTo(Equal(time.Duration(0)))
		})

		It("should handle a non-retryable error correctly", func() {
			// For this test, we can use a standard error that doesn't implement RetryableError
			standardErr := fmt.Errorf("This is a standard error")

			obj := test.NewObject("non-retryable-obj", "default")
			_, result := ctrlerrors.HandleError(ctx, obj, standardErr, recorder)
			Expect(result.RequeueAfter).NotTo(Equal(time.Duration(0)))
		})
	})

	Context("RetryableWithDelayError", func() {
		It("should correctly handle a retryable error with delay", func() {
			specificDelay := 10 * time.Second
			ctrlErr := ctrlerrors.RetryableWithDelayErrorf(specificDelay, "This is a retryable error with delay")
			Expect(ctrlErr.IsRetryable()).To(BeTrue())
			Expect(ctrlErr.Error()).To(Equal("This is a retryable error with delay"))
			Expect(ctrlErr.RetryDelay()).To(Equal(specificDelay))

			obj := test.NewObject("retryable-delay-obj", "default")
			_, result := ctrlerrors.HandleError(ctx, obj, ctrlErr, recorder)
			Expect(result.RequeueAfter).To(BeNumerically(">", specificDelay))
		})

		It("should support custom retryable errors with delay", func() {
			specificDelay := 5 * time.Second
			myErr := &MyRetryableWithDelayError{
				msg:   "Custom retryable with delay error",
				delay: specificDelay,
			}
			Expect(myErr.IsRetryable()).To(BeTrue())
			Expect(myErr.RetryDelay()).To(Equal(specificDelay))
			Expect(myErr.Error()).To(Equal("Custom retryable with delay error"))

			obj := test.NewObject("custom-retryable-delay-obj", "default")
			_, result := ctrlerrors.HandleError(ctx, obj, myErr, recorder)
			Expect(result.RequeueAfter).To(BeNumerically(">", specificDelay))
		})
	})

	Context("Cascading errors", func() {
		It("must select the first known error in the chain", func() {
			blockedErr := ctrlerrors.BlockedErrorf("This is a blocked error")
			wrappedErr1 := errors.Wrapf(blockedErr, "Wrapper 1")
			wrappedErr2 := errors.Wrapf(wrappedErr1, "Wrapper 2")

			obj := test.NewObject("cascading-obj", "default")
			_, result := ctrlerrors.HandleError(ctx, obj, wrappedErr2, recorder)
			Expect(result.RequeueAfter).To(BeNumerically(">", 30*time.Minute))
			condition := obj.GetConditions()[0]
			Expect(condition.Type).To(Equal("Processing"))
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("Blocked"))
			Expect(condition.Message).To(Equal("This is a blocked error"))

			events := recorder.GetEvents(obj)
			Expect(events).To(HaveLen(1))
			Expect(events[0].EventType).To(Equal("Warning"))
			Expect(events[0].Reason).To(Equal("Blocked"))
			Expect(events[0].Message).To(Equal("This is a blocked error"))
		})

	})
})
