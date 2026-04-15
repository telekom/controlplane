// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/telekom/controlplane/projector/internal/metrics"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics", func() {

	// ---------------------------------------------------------------
	// Registration: all five collectors must be registered and resolve
	// to the expected fully-qualified metric name.
	// ---------------------------------------------------------------
	Describe("registration", func() {
		It("registers ReconcileTotal as projector_reconcile_total", func() {
			name := fullyQualifiedName(metrics.ReconcileTotal)
			Expect(name).To(Equal("projector_reconcile_total"))
		})

		It("registers ReconcileDuration as projector_reconcile_duration_seconds", func() {
			name := fullyQualifiedName(metrics.ReconcileDuration)
			Expect(name).To(Equal("projector_reconcile_duration_seconds"))
		})

		It("registers IDResolverLookups as projector_idresolver_lookups_total", func() {
			name := fullyQualifiedName(metrics.IDResolverLookups)
			Expect(name).To(Equal("projector_idresolver_lookups_total"))
		})

		It("registers IDResolverSingleflight as projector_idresolver_singleflight_total", func() {
			name := fullyQualifiedName(metrics.IDResolverSingleflight)
			Expect(name).To(Equal("projector_idresolver_singleflight_total"))
		})

		It("registers DBOperationDuration as projector_db_operation_duration_seconds", func() {
			name := fullyQualifiedName(metrics.DBOperationDuration)
			Expect(name).To(Equal("projector_db_operation_duration_seconds"))
		})
	})

	// ---------------------------------------------------------------
	// Recording & reading: verify that incrementing / observing a
	// metric produces a value that can be read back via testutil.
	// ---------------------------------------------------------------
	Describe("ReconcileTotal", func() {
		It("increments for a given module and outcome", func() {
			metrics.ReconcileTotal.WithLabelValues("zone", metrics.OutcomeSuccess).Inc()

			count := testutil.ToFloat64(
				metrics.ReconcileTotal.WithLabelValues("zone", metrics.OutcomeSuccess),
			)
			Expect(count).To(BeNumerically(">=", 1))
		})
	})

	Describe("ReconcileDuration", func() {
		It("records an observation", func() {
			metrics.ReconcileDuration.WithLabelValues("group", metrics.OutcomeError).Observe(0.042)

			// Verify the histogram collector produces metric families.
			count := testutil.CollectAndCount(metrics.ReconcileDuration)
			Expect(count).To(BeNumerically(">", 0))
		})
	})

	Describe("IDResolverLookups", func() {
		It("increments for cache_hit", func() {
			metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultCacheHit).Inc()

			count := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("zone", metrics.ResultCacheHit),
			)
			Expect(count).To(BeNumerically(">=", 1))
		})

		It("increments for db_miss", func() {
			metrics.IDResolverLookups.WithLabelValues("team", metrics.ResultDBMiss).Inc()

			count := testutil.ToFloat64(
				metrics.IDResolverLookups.WithLabelValues("team", metrics.ResultDBMiss),
			)
			Expect(count).To(BeNumerically(">=", 1))
		})
	})

	Describe("IDResolverSingleflight", func() {
		It("increments for shared=true", func() {
			metrics.IDResolverSingleflight.WithLabelValues("application", "true").Inc()

			count := testutil.ToFloat64(
				metrics.IDResolverSingleflight.WithLabelValues("application", "true"),
			)
			Expect(count).To(BeNumerically(">=", 1))
		})
	})

	Describe("DBOperationDuration", func() {
		It("records an upsert observation", func() {
			metrics.DBOperationDuration.WithLabelValues("zone", metrics.OperationUpsert).Observe(0.005)

			count := testutil.CollectAndCount(metrics.DBOperationDuration)
			Expect(count).To(BeNumerically(">", 0))
		})

		It("records a delete observation", func() {
			metrics.DBOperationDuration.WithLabelValues("team", metrics.OperationDelete).Observe(0.003)

			count := testutil.CollectAndCount(metrics.DBOperationDuration)
			Expect(count).To(BeNumerically(">", 0))
		})
	})

	// ---------------------------------------------------------------
	// Label constants: verify the exported constants have the expected
	// string values (guards against accidental renames).
	// ---------------------------------------------------------------
	Describe("label constants", func() {
		It("defines all reconcile outcome labels", func() {
			Expect(metrics.OutcomeSuccess).To(Equal("success"))
			Expect(metrics.OutcomeSkip).To(Equal("skip"))
			Expect(metrics.OutcomeDependencyMissing).To(Equal("dependency_missing"))
			Expect(metrics.OutcomeDeleteKeyLost).To(Equal("delete_key_lost"))
			Expect(metrics.OutcomeDeleteSuccess).To(Equal("delete_success"))
			Expect(metrics.OutcomeError).To(Equal("error"))
		})

		It("defines all IDResolver result labels", func() {
			Expect(metrics.ResultCacheHit).To(Equal("cache_hit"))
			Expect(metrics.ResultNegCacheHit).To(Equal("neg_cache_hit"))
			Expect(metrics.ResultDBHit).To(Equal("db_hit"))
			Expect(metrics.ResultDBMiss).To(Equal("db_miss"))
		})

		It("defines all DB operation labels", func() {
			Expect(metrics.OperationUpsert).To(Equal("upsert"))
			Expect(metrics.OperationDelete).To(Equal("delete"))
		})
	})
})

// fullyQualifiedName extracts the FQ metric name from a collector by
// inspecting its Desc. This works for both CounterVec and HistogramVec.
func fullyQualifiedName(c prometheus.Collector) string {
	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)
	desc := <-ch
	if desc == nil {
		return ""
	}
	// desc.String() returns e.g.:
	//   Desc{fqName: "projector_reconcile_total", help: "...", ...}
	s := desc.String()
	// Extract the fqName value between the first pair of quotes.
	const prefix = `fqName: "`
	i := strings.Index(s, prefix)
	if i < 0 {
		return s
	}
	s = s[i+len(prefix):]
	j := strings.Index(s, `"`)
	if j < 0 {
		return s
	}
	return s[:j]
}
