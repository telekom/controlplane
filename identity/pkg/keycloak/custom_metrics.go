// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package keycloak

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	keycloakRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "keycloakRequests_total",
			Help: "How many HTTP requests to keycloak processed, partitioned by status code and HTTP method.",
		},
		[]string{"code", "method", "func"})

	keycloakRequestsFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "keycloakRequests_failures",
			Help: "Number of failed HTTP requests to keycloak",
		},
		[]string{"status"},
	)

	keycloakRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "keycloakRequests_duration_seconds",
			Help:    "Duration of keycloak requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "func"})
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(keycloakRequests, keycloakRequestsFailures, keycloakRequestDuration)
}

func IncreaseDurationMetrics(start time.Time, lvs ...string) {
	duration := time.Since(start).Seconds()
	keycloakRequestDuration.WithLabelValues(lvs...).Observe(duration)
}

func IncreaseStatusMetrics(lvs ...string) {
	keycloakRequests.WithLabelValues(lvs...).Inc()
}

func IncreaseErrorMetrics() {
	keycloakRequestsFailures.WithLabelValues("error").Inc()
}
