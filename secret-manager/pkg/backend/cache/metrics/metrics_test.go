// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("Cache Metrics", Ordered, func() {

	Context("Register Metrics", Ordered, func() {

		It("should record cache hits", func() {
			// Record a cache hit
			RecordCacheHit("get", "")

			// Verify metric exists with correct labels
			metrics, err := prometheus.DefaultGatherer.Gather()
			Expect(err).NotTo(HaveOccurred())

			var hitFound bool
			for _, m := range metrics {
				if m.GetName() == "cache_access_total" {
					for _, metric := range m.GetMetric() {
						var isHit, emptyReason bool

						for _, label := range metric.GetLabel() {
							if label.GetName() == "result" && label.GetValue() == "hit" {
								isHit = true
							}
							if label.GetName() == "reason" && label.GetValue() == "" {
								emptyReason = true
							}
							if label.GetName() == "method" && label.GetValue() != "get" {
								isHit = false
							}
						}

						if isHit && emptyReason {
							hitFound = true
							Expect(metric.GetCounter().GetValue()).To(BeNumerically(">", 0))
						}
					}
				}
			}
			Expect(hitFound).To(BeTrue(), "Expected hit metric not found")
		})

		It("should record cache misses with reason", func() {
			// Record cache misses with different reasons
			RecordCacheMiss("set", "expired")
			RecordCacheMiss("set", "not_found")

			// Verify metrics exist with correct labels
			metrics, err := prometheus.DefaultGatherer.Gather()
			Expect(err).NotTo(HaveOccurred())

			expiredFound := false
			notFoundFound := false

			for _, m := range metrics {
				if m.GetName() == "cache_access_total" {
					for _, metric := range m.GetMetric() {
						isMiss := false
						reason := ""

						for _, label := range metric.GetLabel() {
							if label.GetName() == "result" && label.GetValue() == "miss" {
								isMiss = true
							}
							if label.GetName() == "reason" {
								reason = label.GetValue()
							}
							if label.GetName() == "method" && label.GetValue() != "set" {
								isMiss = false
							}
						}

						if isMiss {
							if reason == "expired" {
								expiredFound = true
								Expect(metric.GetCounter().GetValue()).To(BeNumerically(">", 0))
							} else if reason == "not_found" {
								notFoundFound = true
								Expect(metric.GetCounter().GetValue()).To(BeNumerically(">", 0))
							}
						}
					}
				}
			}

			Expect(expiredFound).To(BeTrue(), "Expected miss metric with 'expired' reason not found")
			Expect(notFoundFound).To(BeTrue(), "Expected miss metric with 'not_found' reason not found")
		})

		It("should register metrics only once", func() {
			// Call RegisterMetrics multiple times
			registerMetrics(prometheus.DefaultRegisterer)
			registerMetrics(prometheus.DefaultRegisterer)
			registerMetrics(prometheus.DefaultRegisterer)

			metrics, err := prometheus.DefaultGatherer.Gather()
			Expect(err).NotTo(HaveOccurred())

			// Count our metric
			count := 0
			for _, m := range metrics {
				if m.GetName() == "cache_access_total" {
					count++
				}
			}
			Expect(count).To(Equal(1), "Metric should be registered exactly once")
		})

	})
})
