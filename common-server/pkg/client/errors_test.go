// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}

var _ = Describe("HandleError", func() {
	httpError := func(status int) *HttpError {
		err := HandleError(status, "msg")
		Expect(err).To(BeAssignableToTypeOf(&HttpError{}))
		return err.(*HttpError)
	}

	It("returns nil for an OK status code", func() {
		Expect(HandleError(http.StatusOK, "msg")).To(Succeed())
	})

	DescribeTable("retries server errors without a fixed delay",
		func(status int) {
			err := httpError(status)
			Expect(err.IsRetryable()).To(BeTrue())
			Expect(err.IsBlocked()).To(BeFalse())
			Expect(err.RetryDelay()).To(BeZero())
			Expect(err.StatusCode()).To(Equal(status))
		},
		Entry("conflict", http.StatusConflict),
		Entry("internal server error", http.StatusInternalServerError),
		Entry("bad gateway", http.StatusBadGateway),
		Entry("service unavailable", http.StatusServiceUnavailable),
		Entry("gateway timeout", http.StatusGatewayTimeout),
	)

	It("retries a rate limit after a fixed delay", func() {
		err := httpError(http.StatusTooManyRequests)
		Expect(err.IsRetryable()).To(BeTrue())
		Expect(err.RetryDelay()).To(Equal(RetryDelay))
	})

	It("blocks a bad request", func() {
		Expect(HandleError(http.StatusBadRequest, "msg")).To(Satisfy(func(err error) bool {
			return err.(*HttpError).IsBlocked()
		}))
	})
})
