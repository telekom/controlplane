// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type statusUpdateResult string

const (
	statusUpdateResultUpdated statusUpdateResult = "updated"
	statusUpdateResultSkipped statusUpdateResult = "skipped"
	statusUpdateResultError   statusUpdateResult = "error"
)

var statusUpdatesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "controlplane",
	Subsystem: "controller",
	Name:      "status_updates_total",
	Help:      "Total number of controller status update outcomes.",
}, []string{"result", "cr_type"})

func init() {
	metrics.Registry.MustRegister(statusUpdatesTotal)
}

func recordStatusUpdate(result statusUpdateResult, object runtime.Object, scheme *runtime.Scheme) {
	crType := "unknown"
	if gvk, err := apiutil.GVKForObject(object, scheme); err == nil {
		crType = strings.ToLower(gvk.Kind)
	}
	statusUpdatesTotal.WithLabelValues(string(result), crType).Inc()
}
