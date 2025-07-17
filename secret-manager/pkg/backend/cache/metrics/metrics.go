// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce = sync.Once{}
	// Cache metrics
	cacheAccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_access_total",
			Help: "Total number of cache access attempts",
		},
		[]string{"result", "reason"},
	)
)

func init() {
	registerMetrics(prometheus.DefaultRegisterer)
}

// RegisterMetrics registers all cache-related metrics with Prometheus
func registerMetrics(reg prometheus.Registerer) {
	registerOnce.Do(func() {
		reg.MustRegister(cacheAccess)
	})
}

// RecordCacheHit increments the counter for a successful cache hit
func RecordCacheHit() {
	cacheAccess.WithLabelValues("hit", "").Inc()
}

// RecordCacheMiss increments the counter for a cache miss with the specified reasons like "expired" or "not_found"
func RecordCacheMiss(reason string) {
	cacheAccess.WithLabelValues("miss", reason).Inc()
}
