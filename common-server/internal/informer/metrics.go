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
)

func Register(reg prometheus.Registerer) {
	registerOnce.Do(func() {
		reg.MustRegister(counter)
		reg.MustRegister(queueSize)
		reg.MustRegister(reloads)
	})
}
