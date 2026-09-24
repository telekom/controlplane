// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client_test

import (
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	. "github.com/onsi/gomega"
)

type testPlugin struct {
	id       string
	name     string
	route    *string
	consumer *string
	config   map[string]any
}

func (p *testPlugin) GetId() string             { return p.id }
func (p *testPlugin) SetId(id string)           { p.id = id }
func (p *testPlugin) GetName() string           { return p.name }
func (p *testPlugin) GetRoute() *string         { return p.route }
func (p *testPlugin) GetConsumer() *string      { return p.consumer }
func (p *testPlugin) GetConfig() map[string]any { return p.config }

func ptr[T any](value T) *T {
	return &value
}

// reconcileCount reads the current value of the reconcile outcome counter.
func reconcileCount(entity, outcome string) float64 {
	families, err := metrics.Registry.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, family := range families {
		if family.GetName() != "gateway_kong_reconcile_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["entity"] == entity && labels["outcome"] == outcome {
				return metric.Counter.GetValue()
			}
		}
	}
	return 0
}
