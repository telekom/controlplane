// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package bouncer

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce = sync.Once{}
	queueLength  = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "bouncer_queue_length",
			Help: "Number of items in the queue",
		},
		[]string{"queue"},
	)

	timeInQueue = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bouncer_time_in_queue",
			Help:    "Time spent in the queue",
			Buckets: []float64{0.1, 0.5, 1, 2},
		},
		[]string{"queue", "status"},
	)
)

func RegisterMetrics(reg prometheus.Registerer) {
	registerOnce.Do(func() {
		reg.MustRegister(queueLength)
		reg.MustRegister(timeInQueue)
	})
}
