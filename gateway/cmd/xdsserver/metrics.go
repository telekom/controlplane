// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package xdsserver

import (
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/prometheus/client_golang/prometheus"
)

type callbackMetrics struct {
	requests      *prometheus.CounterVec
	responses     *prometheus.CounterVec
	acks          *prometheus.CounterVec
	nacks         *prometheus.CounterVec
	activeStreams *prometheus.GaugeVec
	unauthorized  *prometheus.CounterVec
}

func newCallbackMetrics(registry prometheus.Registerer) (*callbackMetrics, error) {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}
	m := &callbackMetrics{
		requests:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_xds_requests_total", Help: "Number of xDS discovery requests."}, []string{"type_url"}),
		responses:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_xds_responses_total", Help: "Number of xDS discovery responses."}, []string{"type_url"}),
		acks:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_xds_acks_total", Help: "Number of xDS ACK requests."}, []string{"type_url"}),
		nacks:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_xds_nacks_total", Help: "Number of xDS NACK requests."}, []string{"type_url"}),
		activeStreams: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "gateway_xds_active_streams", Help: "Number of active xDS streams."}, []string{"type_url"}),
		unauthorized:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gateway_xds_unauthorized_requests_total", Help: "Number of rejected xDS streams and requests."}, []string{"type_url"}),
	}
	for _, collector := range []prometheus.Collector{m.requests, m.responses, m.acks, m.nacks, m.activeStreams, m.unauthorized} {
		if err := registry.Register(collector); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func metricType(typeURL string) string {
	switch typeURL {
	case "":
		return "ads"
	case resourcev3.ClusterType, resourcev3.EndpointType, resourcev3.ListenerType, resourcev3.RouteType:
		return typeURL
	default:
		return "unknown"
	}
}
