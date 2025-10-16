// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package conjur

import (
	"io"
	"sync"
	"time"

	"github.com/cyberark/conjur-api-go/conjurapi"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once
	histogram    = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_client_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
	}, []string{"client", "method", "status"})
)

const (
	name = "conjur"
)

func Register(reg prometheus.Registerer) {
	registerOnce.Do(func() {
		reg.MustRegister(histogram)
	})
}

func init() {
	Register(prometheus.DefaultRegisterer)
}

var _ ConjurAPI = (*ConjurApiMetrics)(nil)

type ConjurApiMetrics struct {
	Client ConjurAPI
}

func NewConjurApiMetrics(client ConjurAPI) *ConjurApiMetrics {
	return &ConjurApiMetrics{
		Client: client,
	}
}

// AddSecret implements ConjurAPI.
func (c *ConjurApiMetrics) AddSecret(variableID string, value string) error {
	start := time.Now()
	err := c.Client.AddSecret(variableID, value)
	status := "ok"
	if err != nil {
		status = "error"
	}
	histogram.WithLabelValues(name, "AddSecret", status).Observe(time.Since(start).Seconds())
	return err
}

// LoadPolicy implements ConjurAPI.
func (c *ConjurApiMetrics) LoadPolicy(mode conjurapi.PolicyMode, path string, reader io.Reader) (*conjurapi.PolicyResponse, error) {
	start := time.Now()
	resp, err := c.Client.LoadPolicy(mode, path, reader)
	status := "ok"
	if err != nil {
		status = "error"
	}
	histogram.WithLabelValues(name, "LoadPolicy", status).Observe(time.Since(start).Seconds())
	return resp, err
}

// RetrieveBatchSecrets implements ConjurAPI.
func (c *ConjurApiMetrics) RetrieveBatchSecrets(variableIDs []string) (map[string][]byte, error) {
	start := time.Now()
	res, err := c.Client.RetrieveBatchSecrets(variableIDs)
	status := "ok"
	if err != nil {
		status = "error"
	}
	histogram.WithLabelValues(name, "RetrieveBatchSecrets", status).Observe(time.Since(start).Seconds())
	return res, err
}

// RetrieveSecret implements ConjurAPI.
func (c *ConjurApiMetrics) RetrieveSecret(variableID string) ([]byte, error) {
	start := time.Now()
	res, err := c.Client.RetrieveSecret(variableID)
	status := "ok"
	if err != nil {
		status = "error"
	}
	histogram.WithLabelValues(name, "RetrieveSecret", status).Observe(time.Since(start).Seconds())
	return res, err
}

// RetrieveSecretWithVersion implements ConjurAPI.
func (c *ConjurApiMetrics) RetrieveSecretWithVersion(variableID string, version int) ([]byte, error) {
	start := time.Now()
	res, err := c.Client.RetrieveSecretWithVersion(variableID, version)
	status := "ok"
	if err != nil {
		status = "error"
	}
	histogram.WithLabelValues(name, "RetrieveSecretWithVersion", status).Observe(time.Since(start).Seconds())
	return res, err
}
