package client

import (
	"net/http"
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
	inner              HttpRequestDoer
	clientName         string
	pathReplacePattern *regexp.Regexp
}

func Register(reg prometheus.Registerer) {
	registerOnce.Do(func() {
		reg.MustRegister(histogram)
	})
}

// WithMetrics wraps an HttpRequestDoer and records metrics for HTTP requests.
// clientName is added as a label to the metrics.
// pathReplacePattern is a regex pattern used to replace dynamic parts of the URL path in the metrics.
// e.g. `\/api\/v1\/users\/(?P<resourceId>.*)` will replace "/api/v1/users/123" with "/api/v1/users/resourceId".
// If no named groups are found, the original path will be used.
func WithMetrics(inner HttpRequestDoer, clientName, pathReplacePattern string) HttpRequestDoer {
	c := &clientWrapper{
		inner:      inner,
		clientName: clientName,
	}

	if pathReplacePattern != "" {
		c.pathReplacePattern = regexp.MustCompile(pathReplacePattern)
	}
	Register(prometheus.DefaultRegisterer)

	return c
}

func (c *clientWrapper) currentTime() time.Time {
	return time.Now()
}

func (c *clientWrapper) Do(req *http.Request) (*http.Response, error) {
	startTime := c.currentTime()

	res, err := c.inner.Do(req)

	elapsed := time.Since(startTime).Seconds()
	method := req.Method
	path := ReplacePath(c.pathReplacePattern, req.URL.Path)
	var status string

	if res != nil {
		status = strconv.Itoa(res.StatusCode)
	} else {
		status = "error"
	}
	histogram.WithLabelValues(c.clientName, method, path, status).Observe(elapsed)

	return res, err
}

// ReplacePath replaces the named groups in the path with their corresponding names.
// If no named groups are found, the original path is returned.
// If the regex is nil, the original path is returned.
// The named groups are replaced with their names, and the rest of the path is preserved.
// For example, if the regex is `\/api\/v1\/users\/(?P<redacted>.*)` and the path is `/api/v1/users/123`,
// the result will be `/api/v1/users/redacted`.
func ReplacePath(re *regexp.Regexp, path string) string {
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
	return sb.String()
}
