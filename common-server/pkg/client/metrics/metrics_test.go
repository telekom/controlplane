// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	prometheusdto "github.com/prometheus/client_model/go"
	client "github.com/telekom/controlplane/common-server/pkg/client/metrics"
)

var _ = Describe("Client Metrics", func() {

	Context("Setup", func() {
		It("should register the client wrapper", func() {
			httpClient := http.DefaultClient

			httpDoer := client.WithMetrics(httpClient, client.WithClientName("testClient"))
			Expect(httpDoer).ToNot(BeNil())
		})
	})

	Context("Collecting Metrics", func() {
		ExpectedMetricName := "http_client_request_duration_seconds"

		It("should collect metrics when successful request is made", func() {
			httpClient := &mockClient{}

			httpDoer := client.WithMetrics(httpClient, client.WithClientName("testClient"))
			Expect(httpDoer).ToNot(BeNil())

			req := httptest.NewRequest("GET", "/test/path", nil)
			res, err := httpDoer.Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusOK))

			metrics, err := prometheus.DefaultGatherer.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(metrics).ToNot(BeEmpty())

			m := findMetric(metrics, ExpectedMetricName, "testClient")
			Expect(m).ToNot(BeNil())
			Expect(m.GetLabel()[0].GetName()).To(Equal("client"))
			Expect(m.GetLabel()[0].GetValue()).To(Equal("testClient"))

			Expect(m.GetLabel()[1].GetName()).To(Equal("method"))
			Expect(m.GetLabel()[1].GetValue()).To(Equal("GET"))

			Expect(m.GetLabel()[2].GetName()).To(Equal("path"))
			Expect(m.GetLabel()[2].GetValue()).To(Equal("/test/path"))

			Expect(m.GetLabel()[3].GetName()).To(Equal("status"))
			Expect(m.GetLabel()[3].GetValue()).To(Equal("200"))
			Expect(m.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))

			foundMetric := false
			for _, m := range metrics {
				if m.GetName() == ExpectedMetricName {
					foundMetric = true

					Expect(m.GetType().String()).To(Equal("HISTOGRAM"))
					Expect(m.GetHelp()).To(Equal("Duration of HTTP requests in seconds"))
				}
			}
			Expect(foundMetric).To(BeTrue(), "Expected metric %s not found", ExpectedMetricName)
		})

	})
})

var _ = Describe("ReplacePath", func() {

	It("should replace the path with the placeholder", func() {
		pattern := `\/api\/v1\/users\/(?P<redacted>.*)`
		path := "/api/v1/users/123"

		re := regexp.MustCompile(pattern)
		replacedPath := client.NewReplacePath(re)(path)
		Expect(replacedPath).To(Equal("/api/v1/users/redacted"))
	})

	It("should return the original path if no pattern is provided", func() {
		path := "/test/foo123/subpath/456"

		replacedPath := client.NewReplacePath(nil)(path)
		Expect(replacedPath).To(Equal(path))
	})

	It("should return the original path if the pattern does not match", func() {
		pattern := `^.*\/test\/(.*)\/subpath\/(?P<redacted>.*)$`
		path := "/foo123/subpath/456"

		re := regexp.MustCompile(pattern)
		replacedPath := client.NewReplacePath(re)(path)
		Expect(replacedPath).To(Equal(path))
	})

	It("should return the original path if no named group is provided", func() {
		pattern := `\/api\/v1\/(users|products)\/(?P<resourceId>.*)`

		re := regexp.MustCompile(pattern)

		replacedPath := client.NewReplacePath(re)("/api/v1/users/123")
		Expect(replacedPath).To(Equal("/api/v1/users/resourceId"))

		replacedPath = client.NewReplacePath(re)("/api/v1/products/456")
		Expect(replacedPath).To(Equal("/api/v1/products/resourceId"))
	})

	It("should support the redacted part in the middle of the path", func() {
		pattern := `\/api\/v1\/(?P<redacted>.*)\/users\/.*`
		path := "/api/v1/some/path/users/123"

		re := regexp.MustCompile(pattern)
		replacedPath := client.NewReplacePath(re)(path)
		Expect(replacedPath).To(Equal("/api/v1/redacted/users/123"))
	})

})

var _ = Describe("Error Categorization", func() {
	var errorReturn error
	httpClient := &mockClient{
		do: func(req *http.Request) (*http.Response, error) {
			return nil, errorReturn
		},
	}

	testErrorCategorization := func(clientName string, expectedStatus string) {
		httpDoer := client.WithMetrics(httpClient, client.WithClientName(clientName))
		req := httptest.NewRequest("GET", "/test", nil)
		_, err := httpDoer.Do(req)

		if errorReturn != nil {
			Expect(err).To(HaveOccurred())
		}

		metrics, err := prometheus.DefaultGatherer.Gather()
		Expect(err).ToNot(HaveOccurred())

		status := findMetricStatus(metrics, "http_client_request_duration_seconds", clientName)
		Expect(status).To(Equal(expectedStatus))
	}

	Context("Timeout Errors", func() {
		It("should categorize context.DeadlineExceeded as error:timeout", func() {
			errorReturn = context.DeadlineExceeded
			testErrorCategorization("timeout-test", "error:timeout")
		})

		It("should categorize os.ErrDeadlineExceeded as error:timeout", func() {
			errorReturn = os.ErrDeadlineExceeded
			testErrorCategorization("timeout-test2", "error:timeout")
		})

		It("should categorize net.Error timeout as error:timeout", func() {
			errorReturn = &timeoutError{}
			testErrorCategorization("timeout-test3", "error:timeout")
		})
	})

	Context("Canceled Requests", func() {
		It("should categorize context.Canceled as error:canceled", func() {
			errorReturn = context.Canceled
			testErrorCategorization("canceled-test", "error:canceled")
		})
	})

	Context("TLS Errors", func() {
		It("should categorize tls.RecordHeaderError as error:tls", func() {
			errorReturn = tls.RecordHeaderError{Msg: "bad record MAC"}
			testErrorCategorization("tls-test", "error:tls")
		})

		It("should categorize errors containing 'tls' as error:tls", func() {
			errorReturn = fmt.Errorf("tls handshake failed")
			testErrorCategorization("tls-test2", "error:tls")
		})

		It("should categorize errors containing 'certificate' as error:tls", func() {
			errorReturn = fmt.Errorf("certificate verify failed")
			testErrorCategorization("tls-test3", "error:tls")
		})
	})

	Context("Connection Refused Errors", func() {
		It("should categorize dial OpError as error:connection_refused", func() {
			errorReturn = &net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")}
			testErrorCategorization("conn-refused-test", "error:connection_refused")
		})

		It("should categorize errors containing 'connection refused' as error:connection_refused", func() {
			errorReturn = fmt.Errorf("dial tcp: connection refused")
			testErrorCategorization("conn-refused-test2", "error:connection_refused")
		})
	})

	Context("DNS Errors", func() {
		It("should categorize net.DNSError as error:dns", func() {
			errorReturn = &net.DNSError{}
			testErrorCategorization("dns-test", "error:dns")
		})
	})

	Context("Generic Errors", func() {
		It("should categorize unknown errors as error", func() {
			errorReturn = fmt.Errorf("some unknown error")
			testErrorCategorization("generic-test", "error")
		})
	})

	Context("No Response", func() {
		It("should categorize nil error with nil response as error:no_response", func() {
			errorReturn = nil
			testErrorCategorization("no-response-test", "error:no_response")
		})
	})
})

func findMetric(metrics []*prometheusdto.MetricFamily, metricName, clientName string) *prometheusdto.Metric {
	for _, m := range metrics {
		if m.GetName() == metricName {
			for _, metric := range m.GetMetric() {
				labels := metric.GetLabel()

				for _, label := range labels {
					if label.GetName() == "client" && label.GetValue() == clientName {
						return metric
					}
				}

			}
		}
	}
	return nil
}

// Helper function to find the status label value for a specific client
func findMetricStatus(metrics []*prometheusdto.MetricFamily, metricName, clientName string) string {
	metric := findMetric(metrics, metricName, clientName)
	if metric != nil {
		for _, label := range metric.GetLabel() {
			if label.GetName() == "status" {
				return label.GetValue()
			}
		}
	}
	return ""
}

// timeoutError implements net.Error with Timeout() returning true
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout error" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
