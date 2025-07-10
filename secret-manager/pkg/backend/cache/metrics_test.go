// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package cache_test

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/telekom/controlplane/secret-manager/pkg/backend/cache"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache Metrics", Ordered, func() {

	Context("Register Metrics", func() {
		var registry *prometheus.Registry

		BeforeAll(func() {
			registry = prometheus.NewRegistry()
		})

		It("should register the metrics", func() {
			cache.RegisterMetrics(registry)

			metrics, err := registry.Gather()
			Expect(err).NotTo(HaveOccurred())

			// Find our metric in the gathered metrics
			var foundMetric bool
			for _, m := range metrics {
				if m.GetName() == "cache_access_total" {
					foundMetric = true
					Expect(m.GetType().String()).To(Equal("COUNTER"))
					Expect(m.GetHelp()).To(Equal("Total number of cache access attempts"))
				}
			}
			Expect(foundMetric).To(BeTrue(), "Expected metric 'cache_access_total' not found")
		})

		It("should register metrics only once", func() {
			// Call RegisterMetrics multiple times
			cache.RegisterMetrics(registry)
			cache.RegisterMetrics(registry)
			cache.RegisterMetrics(registry)

			metrics, err := registry.Gather()
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

		It("should record cache hits", func() {
			// Record a cache hit
			cache.RecordCacheHit()

			// Verify metric exists with correct labels
			metrics, err := registry.Gather()
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
			cache.RecordCacheMiss("expired")
			cache.RecordCacheMiss("not_found")

			// Verify metrics exist with correct labels
			metrics, err := registry.Gather()
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
	})
})
