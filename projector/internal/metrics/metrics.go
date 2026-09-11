// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package metrics defines and registers custom Prometheus metrics for the
// projector. All metric variables are registered with the controller-runtime
// metrics registry and are automatically served on the existing /metrics
// endpoint.
//
// Recording is performed at call sites (reconciler, IDResolver, repositories);
// this package only handles definition and registration.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Reconcile outcome labels. These classify the result of each reconciliation
// and are used as the "outcome" label on reconcile metrics.
const (
	OutcomeSuccess           = "success"
	OutcomeSkip              = "skip"
	OutcomeDependencyMissing = "dependency_missing"
	OutcomeDeleteKeyLost     = "delete_key_lost"
	OutcomeDeleteSuccess     = "delete_success"
	OutcomeError             = "error"
)

// IDResolver lookup result labels. These classify the cache/DB decision point
// for each Find* call and are used as the "result" label on IDResolver metrics.
const (
	ResultCacheHit    = "cache_hit"
	ResultNegCacheHit = "neg_cache_hit"
	ResultDBHit       = "db_hit"
	ResultDBMiss      = "db_miss"
)

// DB operation labels.
const (
	OperationUpsert = "upsert"
	OperationDelete = "delete"
)

var (
	// ReconcileTotal counts the total number of reconciliations, broken down
	// by module and outcome. This complements the built-in
	// controller_runtime_reconcile_total metric by providing domain-specific
	// outcome classification (e.g., dependency_missing vs generic error).
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "projector",
			Name:      "reconcile_total",
			Help:      "Total number of reconciliations by module and outcome.",
		},
		[]string{"module", "outcome"},
	)

	// ReconcileDuration observes the latency distribution of each
	// reconciliation, broken down by module and outcome. This complements
	// the built-in controller_runtime_reconcile_time_seconds by adding an
	// outcome dimension so you can see latency per error class.
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "projector",
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of reconciliations by module and outcome.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"module", "outcome"},
	)

	// IDResolverLookups counts IDResolver lookups by entity type and result
	// (cache_hit, neg_cache_hit, db_hit, db_miss).
	IDResolverLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "projector",
			Name:      "idresolver_lookups_total",
			Help:      "Total IDResolver lookups by entity type and result.",
		},
		[]string{"entity_type", "result"},
	)

	// IDResolverSingleflight counts singleflight coalescing events by entity
	// type and whether the result was shared from another in-flight caller.
	// This metric will only emit values after IDResolver Hardening (Phase 4)
	// is completed.
	IDResolverSingleflight = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "projector",
			Name:      "idresolver_singleflight_total",
			Help:      "Total IDResolver singleflight events by entity type and shared status.",
		},
		[]string{"entity_type", "shared"},
	)

	// DBOperationDuration observes the latency distribution of database
	// operations (upsert, delete) per module.
	DBOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "projector",
			Name:      "db_operation_duration_seconds",
			Help:      "Duration of database operations by module and operation type.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"module", "operation"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		IDResolverLookups,
		IDResolverSingleflight,
		DBOperationDuration,
	)
}
