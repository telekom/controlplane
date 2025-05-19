// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/telekom/controlplane/common-server/pkg/problems"
)

var (
	registerOnce sync.Once
	histogram    = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds",
		Buckets: []float64{0.01, 0.1, 0.5, 1, 2, 5},
	}, []string{"method", "path", "status"})
)

type Skipper func(c *fiber.Ctx) bool

func NewMiddleware(reg prometheus.Registerer, skipper Skipper) fiber.Handler {
	if reg == nil {
		panic("prometheus.Registerer is nil")
	}

	registerOnce.Do(func() {
		reg.MustRegister(histogram)
	})

	return func(c *fiber.Ctx) error {
		if skipper(c) {
			return c.Next()
		}

		start := time.Now()
		err := c.Next()
		elapsed := float64(time.Since(start).Nanoseconds()) / 1e9
		method := utils.CopyString(c.Method())
		path := utils.CopyString(c.Route().Path)
		if path == "/" {
			path = utils.CopyString(c.Path())
		}
		status, ok := getStatusCodeOnErr(err)
		if !ok {
			status = strconv.Itoa(c.Response().StatusCode())
		}

		histogram.WithLabelValues(method, path, status).Observe(elapsed)
		return err
	}
}

var DefaultSkipper = func(c *fiber.Ctx) bool {
	return c.Path() == "/metrics" || c.Path() == "/healthz" || c.Path() == "/readyz"
}

func NewForApp(app *fiber.App, reg prometheus.Registerer, skipper Skipper) {
	if skipper == nil {
		skipper = DefaultSkipper
	}

	app.Use(NewMiddleware(reg, skipper))
	gatherer, ok := reg.(prometheus.Gatherer)
	if !ok {
		panic("prometheus.Registerer is not a Gatherer")
	}

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
		Timeout:           1 * time.Second,
		EnableOpenMetrics: true,
	})))

}

func getStatusCodeOnErr(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var p problems.Problem
	if errors.As(err, &p) {
		return strconv.Itoa(p.Code()), true
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return strconv.Itoa(fe.Code), true
	}
	return "500", true
}
