// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	registerOnce sync.Once
	histogram    = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_client_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
	}, []string{"client", "method", "path", "status"})
)

type HttpRequestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type clientWrapper struct {
	inner       HttpRequestDoer
	clientName  string
	replaceFunc ReplaceFunc
}

func Register(reg prometheus.Registerer) {
	registerOnce.Do(func() {
		reg.MustRegister(histogram)
	})
}

type ReplaceFunc func(path string) string

type Options struct {
	ClientName         string
	PathReplacePattern string
	ReplaceFunc        ReplaceFunc
}

type Option func(o *Options)

func WithClientName(name string) Option {
	return func(o *Options) {
		o.ClientName = name
	}
}

func WithReplaceFunc(replaceFunc ReplaceFunc) Option {
	return func(o *Options) {
		o.ReplaceFunc = replaceFunc
	}
}

const (
	ReplacePatternUID = `[a-zA-Z0-9-]{36}`
)

func WithReplacePatterns(patterns ...string) Option {
	return func(o *Options) {
		if len(patterns) == 0 {
			o.ReplaceFunc = nil
			return
		}
		re := regexp.MustCompile(strings.Join(patterns, "|"))
		o.ReplaceFunc = func(path string) string {
			new := re.ReplaceAllString(path, "redacted")
			return new
		}
	}
}

// WithMetrics wraps an HttpRequestDoer and records metrics for HTTP requests.
// clientName is added as a label to the metrics.
// pathReplacePattern is a regex pattern used to replace dynamic parts of the URL path in the metrics.
// e.g. `\/api\/v1\/users\/(?P<resourceId>.*)` will replace "/api/v1/users/123" with "/api/v1/users/resourceId".
// If no named groups are found, the original path will be used.
func WithMetrics(inner HttpRequestDoer, opts ...Option) HttpRequestDoer {
	options := &Options{
		ClientName: "default",
	}
	for _, opt := range opts {
		opt(options)
	}

	c := &clientWrapper{
		inner:       inner,
		clientName:  options.ClientName,
		replaceFunc: options.ReplaceFunc,
	}

	if options.PathReplacePattern != "" {
		c.replaceFunc = NewReplacePath(regexp.MustCompile(options.PathReplacePattern))
	}
	Register(prometheus.DefaultRegisterer)

	return c
}

func (c *clientWrapper) currentTime() time.Time {
	return time.Now()
}

func categorizeError(err error) string {
	if err == nil {
		return "error:no_response"
	}

	// timeout
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return "error:timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "error:timeout"
	}

	// canceled requests
	if errors.Is(err, context.Canceled) {
		return "error:canceled"
	}

	// tls
	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return "error:tls"
	}
	if strings.Contains(err.Error(), "tls") || strings.Contains(err.Error(), "certificate") {
		return "error:tls"
	}

	// misc connection refused
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return "error:connection_refused"
		}
	}
	if strings.Contains(err.Error(), "connection refused") {
		return "error:connection_refused"
	}

	// dns
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "error:dns"
	}

	return "error"
}

func (c *clientWrapper) Do(req *http.Request) (*http.Response, error) {
	startTime := c.currentTime()

	res, err := c.inner.Do(req)

	elapsed := time.Since(startTime).Seconds()
	method := req.Method
	var path string
	if c.replaceFunc != nil {
		path = c.replaceFunc(req.URL.Path)
	} else {
		path = req.URL.Path
	}
	var status string

	if res != nil {
		status = strconv.Itoa(res.StatusCode)
	} else {
		status = categorizeError(err)
	}
	histogram.WithLabelValues(c.clientName, method, path, status).Observe(elapsed)

	return res, err
}

// NewReplacePath replaces the named groups in the path with their corresponding names.
// If no named groups are found, the original path is returned.
// If the regex is nil, the original path is returned.
// The named groups are replaced with their names, and the rest of the path is preserved.
// For example, if the regex is `\/api\/v1\/users\/(?P<redacted>.*)` and the path is `/api/v1/users/123`,
// the result will be `/api/v1/users/redacted`.
func NewReplacePath(re *regexp.Regexp) ReplaceFunc {
	return func(path string) string {
		if re == nil {
			return path
		}
		matches := re.FindStringSubmatchIndex(path)
		if len(matches) < 4 {
			return path
		}
		names := re.SubexpNames()
		if len(names) == 0 {
			return path
		}
		var sb strings.Builder
		idx := 0
		for i := 2; i < len(matches); i += 2 {
			start, end := idx, matches[i]
			if start < 0 || end < 0 {
				break
			}
			idx = matches[i+1]
			sb.WriteString(path[start:end])
			placeholder := names[i/2]
			if placeholder == "" {
				sb.WriteString(path[matches[i]:matches[i+1]])
			} else {
				sb.WriteString(placeholder)
			}
		}
		if idx < len(path) {
			sb.WriteString(path[idx:])
		}

		return sb.String()
	}
}
