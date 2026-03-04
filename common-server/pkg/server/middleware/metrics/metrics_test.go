// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/problems"
)

// statusCoderErr is a test error that implements the StatusCoder interface.
type statusCoderErr struct {
	code int
	msg  string
}

func (e *statusCoderErr) Error() string { return e.msg }
func (e *statusCoderErr) Code() int     { return e.code }

var _ = Describe("getStatusCodeOnErr", func() {

	It("should return empty string and false for nil error", func() {
		status, ok := getStatusCodeOnErr(nil)
		Expect(ok).To(BeFalse())
		Expect(status).To(BeEmpty())
	})

	It("should extract status code from problems.Problem", func() {
		status, ok := getStatusCodeOnErr(problems.NotFound())
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("404"))
	})

	It("should extract status code from problems.BadRequest", func() {
		status, ok := getStatusCodeOnErr(problems.BadRequest("invalid"))
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("400"))
	})

	It("should extract status code from fiber.Error", func() {
		status, ok := getStatusCodeOnErr(fiber.ErrBadGateway)
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("502"))
	})

	It("should extract status code from StatusCoder", func() {
		err := &statusCoderErr{code: 422, msg: "unprocessable entity"}
		status, ok := getStatusCodeOnErr(err)
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("422"))
	})

	It("should return 503 for context.DeadlineExceeded", func() {
		status, ok := getStatusCodeOnErr(context.DeadlineExceeded)
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("503"))
	})

	It("should return 500 for unknown errors", func() {
		status, ok := getStatusCodeOnErr(errors.New("something went wrong"))
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("500"))
	})

	It("should extract status code from wrapped problems.Problem", func() {
		wrapped := fmt.Errorf("handler failed: %w", problems.Conflict("duplicate"))
		status, ok := getStatusCodeOnErr(wrapped)
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("409"))
	})

	It("should extract status code from wrapped fiber.Error", func() {
		wrapped := fmt.Errorf("request failed: %w", fiber.ErrTooManyRequests)
		status, ok := getStatusCodeOnErr(wrapped)
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("429"))
	})

	It("should extract status code from wrapped StatusCoder", func() {
		err := &statusCoderErr{code: 503, msg: "service unavailable"}
		wrapped := fmt.Errorf("upstream: %w", err)
		status, ok := getStatusCodeOnErr(wrapped)
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("503"))
	})

	It("should extract status code from wrapped context.DeadlineExceeded", func() {
		wrapped := fmt.Errorf("timed out: %w", context.DeadlineExceeded)
		status, ok := getStatusCodeOnErr(wrapped)
		Expect(ok).To(BeTrue())
		Expect(status).To(Equal("503"))
	})
})
