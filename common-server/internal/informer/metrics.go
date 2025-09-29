// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once

	counter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "informer_events_total",
		Help: "Total number of informer events processed",
	}, []string{"informer", "event_type", "error"},
	)

	queueSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "informer_queue_size",
		Help: "Current size of the informer work queue",
	}, []string{"informer"},
	)

	reloads = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "informer_reloads_total",
		Help: "Total number of informer reloads",
	}, []string{"informer"},
	)

	// Additional metrics for performance monitoring
	activeWorkers = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "informer_active_workers",
		Help: "Number of active worker goroutines",
	}, []string{"informer"},
	)

	watchLoopIterations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "informer_watch_loop_iterations_total",
		Help: "Total number of watch loop iterations",
	}, []string{"informer"},
	)

	eventProcessingLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "informer_event_processing_latency_seconds",
		Help:    "Latency of processing events from watch",
		Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 2, 5, 10},
	}, []string{"informer"},
	)

	listOperations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "informer_list_operations_total",
		Help: "Total number of list operations performed",
	}, []string{"informer"},
	)

	queueWaitTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "informer_queue_wait_time_seconds",
		Help:    "Time events spend waiting in queue",
		Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 5, 10, 30},
	}, []string{"informer"},
	)
)

func Register(reg prometheus.Registerer) {
	registerOnce.Do(func() {
		reg.MustRegister(counter)
		reg.MustRegister(queueSize)
		reg.MustRegister(reloads)
		reg.MustRegister(activeWorkers)
		reg.MustRegister(watchLoopIterations)
		reg.MustRegister(eventProcessingLatency)
		reg.MustRegister(listOperations)
		reg.MustRegister(queueWaitTime)
	})
}
