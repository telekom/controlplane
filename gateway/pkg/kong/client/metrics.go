// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// kongReconcileTotal records whether a reconciliation wrote to Kong. A skipped
// write leaves no trace in the HTTP client metrics, which is why it is counted
// here; request latency and status codes are already covered by
// http_client_request_duration_seconds.
var kongReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "gateway",
	Subsystem: "kong",
	Name:      "reconcile_total",
	Help:      "Kong entity reconciliation outcomes.",
}, []string{"entity", "outcome"})

func init() {
	metrics.Registry.MustRegister(kongReconcileTotal)
}
