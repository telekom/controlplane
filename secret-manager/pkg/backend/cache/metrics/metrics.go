// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type CacheSizeFunc func() float64

var (
	registerOnce = sync.Once{}
	// Cache metrics
	cacheAccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_access_total",
			Help: "Total number of cache access attempts",
		},
		[]string{"method", "result", "reason"},
	)

	CacheSize prometheus.GaugeFunc
)

func SetCacheSizeFunc(cacheSizeFunc CacheSizeFunc) {
	CacheSize = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "cache_size",
			Help: "Current size of the cache",
		},
		cacheSizeFunc,
	)
}

// RegisterMetrics registers all cache-related metrics with Prometheus
func RegisterMetrics(reg prometheus.Registerer, f CacheSizeFunc) {
	if f == nil {
		f = func() float64 { return -1 }
	}
	SetCacheSizeFunc(f)
	registerOnce.Do(func() {
		reg.MustRegister(cacheAccess)
		reg.MustRegister(CacheSize)
	})
}

// RecordCacheHit increments the counter for a successful cache hit
func RecordCacheHit(method, reason string) {
	cacheAccess.WithLabelValues(method, "hit", reason).Inc()
}

// RecordCacheMiss increments the counter for a cache miss with the specified reasons like "expired" or "not_found"
func RecordCacheMiss(method, reason string) {
	cacheAccess.WithLabelValues(method, "miss", reason).Inc()
}
