// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"errors"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("classifyOAuth2Error", func() {
	retrieveError := func(status int) error {
		return &oauth2.RetrieveError{Response: &http.Response{StatusCode: status}}
	}

	DescribeTable("classifies token endpoint responses",
		func(err error, retryable bool, delay time.Duration) {
			var ae *apiError
			Expect(errors.As(classifyOAuth2Error(err), &ae)).To(BeTrue())
			Expect(ae.IsRetryable()).To(Equal(retryable))
			Expect(ae.IsBlocked()).To(Equal(!retryable))
			Expect(ae.RetryDelay()).To(Equal(delay))
		},
		Entry("rate limit", retrieveError(http.StatusTooManyRequests), true, 3*time.Second),
		Entry("server error", retrieveError(http.StatusBadGateway), true, time.Duration(0)),
		Entry("client error", retrieveError(http.StatusForbidden), false, time.Duration(0)),
		Entry("network error", errors.New("dial tcp: connection refused"), true, time.Duration(0)),
	)
})
