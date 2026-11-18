// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/cyberark/conjur-api-go/conjurapi/response"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

type retryTestClient struct {
	ConjurAPI
	read        func() error
	writeCalls  int
	policyCalls int
	writeErr    error
}

func (c *retryTestClient) RetrieveSecret(string) ([]byte, error) {
	if err := c.read(); err != nil {
		return nil, err
	}
	return []byte("value"), nil
}

func (c *retryTestClient) RetrieveBatchSecrets([]string) (map[string][]byte, error) {
	if err := c.read(); err != nil {
		return nil, err
	}
	return map[string][]byte{"id": []byte("value")}, nil
}

func (c *retryTestClient) RetrieveSecretWithVersion(string, int) ([]byte, error) {
	return c.RetrieveSecret("id")
}

func (c *retryTestClient) AddSecret(string, string) error {
	c.writeCalls++
	return c.writeErr
}

func (c *retryTestClient) LoadPolicy(conjurapi.PolicyMode, string, io.Reader) (*conjurapi.PolicyResponse, error) {
	c.policyCalls++
	return nil, c.writeErr
}

func retryAttemptCounts() map[string]float64 {
	GinkgoHelper()
	const metricName = "secret_manager_conjur_read_retry_attempts_total"
	families, err := prometheus.DefaultGatherer.Gather()
	Expect(err).NotTo(HaveOccurred())
	counts := make(map[string]float64)
	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := metric.GetLabel()
			Expect(labels).To(HaveLen(1))
			Expect(labels[0].GetName()).To(Equal("method"))
			Expect(metric.GetCounter()).NotTo(BeNil())
			counts[labels[0].GetValue()] = metric.GetCounter().GetValue()
		}
	}
	Expect(counts).To(HaveLen(3), "method series for %s", metricName)
	return counts
}

var _ = Describe("Conjur read retries", func() {
	DescribeTable("retries reads",
		func(method string, read func(ConjurAPI) (any, error), expected any) {
			client := &retryTestClient{}
			api := NewRetryingReadAPI(client).(*retryingReadAPI)
			var calls int
			var waits []time.Duration
			api.sleep = func(duration time.Duration) { waits = append(waits, duration) }
			client.read = func() error {
				calls++
				if calls < 3 {
					return fmt.Errorf("wrapped: %w", &response.ConjurError{Code: 429})
				}
				return nil
			}
			retries := retryAttemptCounts()
			result, err := read(api)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(expected))
			Expect(calls).To(Equal(3))
			Expect(waits).To(HaveLen(2))
			Expect(waits[0]).To(BeNumerically(">=", 100*time.Millisecond))
			Expect(waits[0]).To(BeNumerically("<=", 120*time.Millisecond))
			Expect(waits[1]).To(BeNumerically(">=", 200*time.Millisecond))
			Expect(waits[1]).To(BeNumerically("<=", 240*time.Millisecond))
			retries[method] += 2
			Expect(retryAttemptCounts()).To(Equal(retries))
		},
		Entry("single secret", "RetrieveSecret", func(api ConjurAPI) (any, error) {
			return api.RetrieveSecret("id")
		}, []byte("value")),
		Entry("batch secrets", "RetrieveBatchSecrets", func(api ConjurAPI) (any, error) {
			return api.RetrieveBatchSecrets([]string{"id"})
		}, map[string][]byte{"id": []byte("value")}),
		Entry("versioned secret", "RetrieveSecretWithVersion", func(api ConjurAPI) (any, error) {
			return api.RetrieveSecretWithVersion("id", 1)
		}, []byte("value")),
	)

	It("returns the last error after exhaustion", func() {
		lastErr := &response.ConjurError{Code: 429}
		var calls int
		var waits []time.Duration
		client := &retryTestClient{read: func() error {
			calls++
			return lastErr
		}}
		api := NewRetryingReadAPI(client).(*retryingReadAPI)
		api.sleep = func(duration time.Duration) { waits = append(waits, duration) }
		retries := retryAttemptCounts()
		_, err := api.RetrieveSecret("id")
		Expect(err).To(BeIdenticalTo(lastErr))
		Expect(calls).To(Equal(3))
		Expect(waits).To(HaveLen(2))
		retries["RetrieveSecret"] += 2
		Expect(retryAttemptCounts()).To(Equal(retries))
	})

	It("does not count reads that succeed on the first attempt", func() {
		api := NewRetryingReadAPI(&retryTestClient{read: func() error { return nil }})
		retries := retryAttemptCounts()
		_, err := api.RetrieveSecret("id")
		Expect(err).NotTo(HaveOccurred())
		Expect(retryAttemptCounts()).To(Equal(retries))
	})

	It("does not retry permanent errors", func() {
		notFound := &response.ConjurError{Code: 404}
		var calls int
		var waits int
		client := &retryTestClient{read: func() error {
			calls++
			return notFound
		}}
		api := NewRetryingReadAPI(client).(*retryingReadAPI)
		api.sleep = func(time.Duration) { waits++ }
		retries := retryAttemptCounts()
		_, err := api.RetrieveSecret("id")
		Expect(err).To(BeIdenticalTo(notFound))
		Expect(calls).To(Equal(1))
		Expect(waits).To(Equal(0))
		Expect(retryAttemptCounts()).To(Equal(retries))
	})

	It("does not retry server errors", func() {
		serverErr := &response.ConjurError{Code: 503}
		var calls int
		client := &retryTestClient{read: func() error {
			calls++
			return serverErr
		}}
		api := NewRetryingReadAPI(client)
		_, err := api.RetrieveSecret("id")
		Expect(err).To(BeIdenticalTo(serverErr))
		Expect(calls).To(Equal(1))
	})

	It("does not retry mutations", func() {
		client := &retryTestClient{writeErr: &response.ConjurError{Code: 503}}
		var waits int
		api := NewRetryingReadAPI(client).(*retryingReadAPI)
		api.sleep = func(time.Duration) { waits++ }
		err := api.AddSecret("id", "value")
		Expect(err).To(BeIdenticalTo(client.writeErr))
		_, err = api.LoadPolicy(conjurapi.PolicyModePost, "path", strings.NewReader("policy"))
		Expect(err).To(BeIdenticalTo(client.writeErr))
		Expect(client.writeCalls).To(Equal(1))
		Expect(client.policyCalls).To(Equal(1))
		Expect(waits).To(Equal(0))
	})

	DescribeTable("classifies wrapped errors",
		func(err error, retryable bool) {
			Expect(isRetryableReadError(fmt.Errorf("wrapped: %w", err))).To(Equal(retryable))
		},
		Entry("request timeout", &response.ConjurError{Code: 408}, true),
		Entry("rate limit", &response.ConjurError{Code: 429}, true),
		Entry("server error", &response.ConjurError{Code: 500}, false),
		Entry("upper server error boundary", &response.ConjurError{Code: 599}, false),
		Entry("outside server error range", &response.ConjurError{Code: 600}, false),
		Entry("forbidden", &response.ConjurError{Code: 403}, false),
		Entry("temporary DNS failure", &net.DNSError{IsTemporary: true}, true),
		Entry("network timeout", &net.OpError{Err: context.DeadlineExceeded}, true),
		Entry("permanent DNS failure", &net.DNSError{IsNotFound: true}, false),
		Entry("connection reset", syscall.ECONNRESET, true),
		Entry("connection refused", syscall.ECONNREFUSED, true),
		Entry("truncated response", io.ErrUnexpectedEOF, true),
		Entry("cancellation", context.Canceled, false),
		Entry("certificate failure", x509.UnknownAuthorityError{}, false),
		Entry("unrelated error", errors.New("invalid request"), false),
	)
})
