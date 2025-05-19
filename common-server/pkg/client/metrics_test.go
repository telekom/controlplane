// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/telekom/controlplane/common-server/pkg/client"
)

var _ = Describe("Client Metrics", func() {

	Context("Setup", func() {
		It("should register the client wrapper", func() {
			httpClient := http.DefaultClient

			httpDoer := client.WithMetrics(httpClient, "testClient", "")
			Expect(httpDoer).ToNot(BeNil())
		})
	})

	Context("Collecting Metrics", func() {
		ExpectedMetricName := "http_client_request_duration_seconds"

		It("should collect metrics when asuccessful request is made", func() {
			httpClient := &mockClient{}

			httpDoer := client.WithMetrics(httpClient, "testClient", "")
			Expect(httpDoer).ToNot(BeNil())

			req := httptest.NewRequest("GET", "/test/path", nil)
			res, err := httpDoer.Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(res).ToNot(BeNil())
			Expect(res.StatusCode).To(Equal(http.StatusOK))

			metrics, err := prometheus.DefaultGatherer.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(metrics).ToNot(BeEmpty())

			var foundMetric bool
			for _, m := range metrics {
				if m.GetName() == ExpectedMetricName {
					foundMetric = true

					Expect(m.GetType().String()).To(Equal("HISTOGRAM"))
					Expect(m.GetHelp()).To(Equal("Duration of HTTP requests in seconds"))

					Expect(m.GetMetric()).To(HaveLen(1))
					metric := m.GetMetric()[0]
					Expect(metric.GetLabel()).To(HaveLen(4))
					Expect(metric.GetLabel()[0].GetName()).To(Equal("client"))
					Expect(metric.GetLabel()[0].GetValue()).To(Equal("testClient"))

					Expect(metric.GetLabel()[1].GetName()).To(Equal("method"))
					Expect(metric.GetLabel()[1].GetValue()).To(Equal("GET"))

					Expect(metric.GetLabel()[2].GetName()).To(Equal("path"))
					Expect(metric.GetLabel()[2].GetValue()).To(Equal("/test/path"))

					Expect(metric.GetLabel()[3].GetName()).To(Equal("status"))
					Expect(metric.GetLabel()[3].GetValue()).To(Equal("200"))

					Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
				}
			}
			Expect(foundMetric).To(BeTrue(), "Expected metric %s not found", ExpectedMetricName)
		})

		It("should collect metrics when a failed request is made", func() {
			httpClient := &mockClient{
				do: func(req *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("error")
				},
			}

			httpDoer := client.WithMetrics(httpClient, "testClient", "")
			Expect(httpDoer).ToNot(BeNil())

			req := httptest.NewRequest("GET", "/test/path", nil)
			res, err := httpDoer.Do(req)
			Expect(err).To(HaveOccurred())
			Expect(res).To(BeNil())

			metrics, err := prometheus.DefaultGatherer.Gather()
			Expect(err).ToNot(HaveOccurred())
			Expect(metrics).ToNot(BeEmpty())

			var foundMetric bool
			for _, m := range metrics {
				if m.GetName() == ExpectedMetricName {
					foundMetric = true

					Expect(m.GetType().String()).To(Equal("HISTOGRAM"))
					Expect(m.GetHelp()).To(Equal("Duration of HTTP requests in seconds"))

					Expect(m.GetMetric()).To(HaveLen(2))
					metric := m.GetMetric()[1]
					Expect(metric.GetLabel()[0].GetName()).To(Equal("client"))
					Expect(metric.GetLabel()[0].GetValue()).To(Equal("testClient"))

					Expect(metric.GetLabel()[1].GetName()).To(Equal("method"))
					Expect(metric.GetLabel()[1].GetValue()).To(Equal("GET"))

					Expect(metric.GetLabel()[2].GetName()).To(Equal("path"))
					Expect(metric.GetLabel()[2].GetValue()).To(Equal("/test/path"))

					Expect(metric.GetLabel()[3].GetName()).To(Equal("status"))
					Expect(metric.GetLabel()[3].GetValue()).To(Equal("error"))

					Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
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
		replacedPath := client.ReplacePath(re, path)
		Expect(replacedPath).To(Equal("/api/v1/users/redacted"))
	})

	It("should return the original path if no pattern is provided", func() {
		path := "/test/foo123/subpath/456"

		replacedPath := client.ReplacePath(nil, path)
		Expect(replacedPath).To(Equal(path))
	})

	It("should return the original path if the pattern does not match", func() {
		pattern := `^.*\/test\/(.*)\/subpath\/(?P<redacted>.*)$`
		path := "/foo123/subpath/456"

		re := regexp.MustCompile(pattern)
		replacedPath := client.ReplacePath(re, path)
		Expect(replacedPath).To(Equal(path))
	})

	It("should return the original path if no named group is provided", func() {
		pattern := `\/api\/v1\/(users|products)\/(?P<resourceId>.*)`

		re := regexp.MustCompile(pattern)

		replacedPath := client.ReplacePath(re, "/api/v1/users/123")
		Expect(replacedPath).To(Equal("/api/v1/users/resourceId"))

		replacedPath = client.ReplacePath(re, "/api/v1/products/123")
		Expect(replacedPath).To(Equal("/api/v1/products/resourceId"))
	})

})
