// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kong Client Suite")
}

type fakeResponse struct {
	code int
}

func (f *fakeResponse) StatusCode() int {
	return f.code
}

var _ = Describe("CheckStatusCode", func() {
	It("returns nil for an OK status code", func() {
		err := CheckStatusCode(&fakeResponse{code: 200}, 200)
		Expect(err).ToNot(HaveOccurred())
	})

	It("returns nil when status matches one of multiple OK codes", func() {
		err := CheckStatusCode(&fakeResponse{code: 204}, 200, 204, 404)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("429 Too Many Requests", func() {
		var apiErr ApiError

		BeforeEach(func() {
			apiErr = CheckStatusCode(&fakeResponse{code: 429}, 200)
			Expect(apiErr).To(HaveOccurred())
		})

		It("is retryable", func() {
			Expect(apiErr.IsRetryable()).To(BeTrue())
			Expect(apiErr.Retriable()).To(BeTrue())
		})

		It("is not blocked", func() {
			Expect(apiErr.IsBlocked()).To(BeFalse())
		})

		It("has a retry delay", func() {
			Expect(apiErr.RetryDelay()).To(Equal(3 * time.Second))
		})

		It("includes status code in message", func() {
			Expect(apiErr.Error()).To(ContainSubstring("429"))
		})
	})

	Context("5xx Server Error", func() {
		var apiErr ApiError

		BeforeEach(func() {
			apiErr = CheckStatusCode(&fakeResponse{code: 502}, 200)
			Expect(apiErr).To(HaveOccurred())
		})

		It("is retryable", func() {
			Expect(apiErr.IsRetryable()).To(BeTrue())
		})

		It("is not blocked", func() {
			Expect(apiErr.IsBlocked()).To(BeFalse())
		})

		It("has no retry delay", func() {
			Expect(apiErr.RetryDelay()).To(Equal(time.Duration(0)))
		})

		It("includes status code in message", func() {
			Expect(apiErr.Error()).To(ContainSubstring("502"))
		})
	})

	Context("4xx Client Error", func() {
		var apiErr ApiError

		BeforeEach(func() {
			apiErr = CheckStatusCode(&fakeResponse{code: 400}, 200)
			Expect(apiErr).To(HaveOccurred())
		})

		It("is not retryable", func() {
			Expect(apiErr.IsRetryable()).To(BeFalse())
		})

		It("is blocked", func() {
			Expect(apiErr.IsBlocked()).To(BeTrue())
		})

		It("includes status code in message", func() {
			Expect(apiErr.Error()).To(ContainSubstring("400"))
		})
	})
})

// ctrlerrors interface types for duck-type verification.
type blockedError interface {
	IsBlocked() bool
}

type retryableError interface {
	IsRetryable() bool
}

type retryableWithDelayError interface {
	IsRetryable() bool
	RetryDelay() time.Duration
}

var _ = Describe("Duck-typing through error chain", func() {
	It("preserves IsBlocked through fmt.Errorf %%w", func() {
		apiErr := CheckStatusCode(&fakeResponse{code: 400}, 200)
		wrapped := fmt.Errorf("failed to create route: %w", apiErr)

		var be blockedError
		Expect(errors.As(wrapped, &be)).To(BeTrue())
		Expect(be.IsBlocked()).To(BeTrue())
	})

	It("preserves IsRetryable through fmt.Errorf %%w", func() {
		apiErr := CheckStatusCode(&fakeResponse{code: 500}, 200)
		wrapped := fmt.Errorf("failed to create route: %w", apiErr)

		var re retryableError
		Expect(errors.As(wrapped, &re)).To(BeTrue())
		Expect(re.IsRetryable()).To(BeTrue())
	})

	It("preserves RetryDelay through fmt.Errorf %%w for 429", func() {
		apiErr := CheckStatusCode(&fakeResponse{code: 429}, 200)
		wrapped := fmt.Errorf("failed to create route: %w", apiErr)

		var rde retryableWithDelayError
		Expect(errors.As(wrapped, &rde)).To(BeTrue())
		Expect(rde.IsRetryable()).To(BeTrue())
		Expect(rde.RetryDelay()).To(Equal(3 * time.Second))
	})

	It("preserves interfaces through multiple layers of wrapping", func() {
		apiErr := CheckStatusCode(&fakeResponse{code: 502}, 200)
		wrapped1 := fmt.Errorf("inner: %w", apiErr)
		wrapped2 := fmt.Errorf("outer: %w", wrapped1)

		var re retryableError
		Expect(errors.As(wrapped2, &re)).To(BeTrue())
		Expect(re.IsRetryable()).To(BeTrue())
	})
})

var _ = Describe("HandleClientError for OAuth2 token endpoint errors", func() {
	tokenError := func(status int) error {
		retrieveErr := &oauth2.RetrieveError{
			Response: &http.Response{StatusCode: status, Status: http.StatusText(status)},
			Body:     []byte("404 page not found"),
		}
		// the HTTP client reports transport failures as *url.Error
		return &url.Error{Op: "Get", URL: "https://kong.example/admin-api/routes", Err: retrieveErr}
	}

	DescribeTable("retries without a fixed delay so the controller backs off",
		func(status int) {
			err := fmt.Errorf("failed to get route: %w", HandleClientError(tokenError(status)))

			var rde retryableWithDelayError
			Expect(errors.As(err, &rde)).To(BeTrue())
			Expect(rde.IsRetryable()).To(BeTrue())
			Expect(rde.RetryDelay()).To(BeZero())

			var be blockedError
			Expect(errors.As(err, &be)).To(BeTrue())
			Expect(be.IsBlocked()).To(BeFalse())
			Expect(err).To(MatchError(ContainSubstring("OAuth2 token endpoint error")))
		},
		Entry("service unavailable", http.StatusServiceUnavailable),
		Entry("internal server error", http.StatusInternalServerError),
		Entry("bad gateway", http.StatusBadGateway),
		Entry("too many requests", http.StatusTooManyRequests),
	)

	DescribeTable("blocks on rejected client credentials",
		func(status int) {
			var be blockedError
			Expect(errors.As(HandleClientError(tokenError(status)), &be)).To(BeTrue())
			Expect(be.IsBlocked()).To(BeTrue())
		},
		Entry("bad request", http.StatusBadRequest),
		Entry("forbidden", http.StatusForbidden),
	)
})
