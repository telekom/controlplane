// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package changeset

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	checksTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "controlplane",
		Subsystem: "migration",
		Name:      "checks_total",
		Help:      "Reconciles whose backend changes were decided on, by outcome: unchanged, allowed, approved or blocked.",
	}, []string{"operator", "kind", "outcome"})
	diffsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "controlplane",
		Subsystem: "migration",
		Name:      "diffs_total",
		Help:      "Diffs of backend changes, by whether they were on the ignore list.",
	}, []string{"operator", "entity", "outcome"})
)

func init() {
	metrics.Registry.MustRegister(checksTotal, diffsTotal)
}
