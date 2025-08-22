---
sidebar_position: 2
---

# Prometheus Metrics

The Control Plane uses Prometheus for monitoring and metrics collection.

## Overview

[Prometheus](https://prometheus.io/) is an open-source systems monitoring and alerting toolkit. The Control Plane integrates Prometheus for:

- Performance monitoring
- Resource utilization tracking
- SLO/SLI measurements
- Operational insights

## Metric Types

The Control Plane implements various Prometheus metric types:

### Counters

Counters track values that only increase (e.g., request counts, error counts):

```go
var (
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "path", "status"},
    )
)

func init() {
    prometheus.MustRegister(requestsTotal)
}

// Usage
requestsTotal.WithLabelValues("GET", "/files", "200").Inc()
```

### Gauges

Gauges track values that can go up and down (e.g., current active requests):

```go
var (
    activeRequests = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "http_active_requests",
            Help: "Number of active HTTP requests",
        },
    )
)

func init() {
    prometheus.MustRegister(activeRequests)
}

// Usage
activeRequests.Inc()
defer activeRequests.Dec()
```

### Histograms

Histograms track distributions of values (e.g., request durations):

```go
var (
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )
)

func init() {
    prometheus.MustRegister(requestDuration)
}

// Usage
timer := prometheus.NewTimer(requestDuration.WithLabelValues("GET", "/files"))
defer timer.ObserveDuration()
```

## Middleware Integration

The Control Plane integrates Prometheus metrics with Gofiber middleware:

```go
func metricsMiddleware() fiber.Handler {
    return func(c *fiber.Ctx) error {
        startTime := time.Now()
        method := c.Method()
        path := c.Route().Path
        
        // Track active requests
        activeRequests.Inc()
        defer activeRequests.Dec()
        
        // Process request
        err := c.Next()
        
        // Record metrics
        status := strconv.Itoa(c.Response().StatusCode())
        duration := time.Since(startTime).Seconds()
        
        requestsTotal.WithLabelValues(method, path, status).Inc()
        requestDuration.WithLabelValues(method, path).Observe(duration)
        
        return err
    }
}
```

## Service Monitors

The Control Plane uses Prometheus Operator's ServiceMonitor resources to configure scraping:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: file-manager
  namespace: controlplane-system
spec:
  selector:
    matchLabels:
      app: file-manager
  endpoints:
  - port: metrics
    path: /metrics
    interval: 15s
    scheme: http
  namespaceSelector:
    matchNames:
    - controlplane-system
```

## Metrics Endpoint

Each Control Plane service exposes a `/metrics` endpoint:

```go
func setupMetricsEndpoint(app *fiber.App) {
    // Create metrics handler
    promHandler := promhttp.Handler()
    
    // Register metrics endpoint
    app.Get("/metrics", func(c *fiber.Ctx) error {
        handler := adaptor.HTTPHandler(promHandler)
        return handler(c)
    })
}
```

## Default Metrics

The Control Plane includes the following standard metrics:

### HTTP Metrics

- `http_requests_total` - Total request count
- `http_request_duration_seconds` - Request duration histogram
- `http_active_requests` - Current active requests
- `http_response_size_bytes` - Response size histogram

### System Metrics

- `process_cpu_seconds_total` - CPU time spent
- `process_resident_memory_bytes` - Memory usage
- `process_open_fds` - Open file descriptors
- `go_goroutines` - Number of goroutines

### Custom Metrics

The File Manager service includes these custom metrics:

- `file_manager_uploads_total` - Total file uploads
- `file_manager_downloads_total` - Total file downloads
- `file_manager_upload_errors_total` - Upload error count
- `file_manager_download_errors_total` - Download error count
- `file_manager_file_size_bytes` - File size histogram

## Grafana Dashboards

The Control Plane provides pre-built Grafana dashboards for monitoring:

- Controller overview metrics
- Resource utilization
- Controller runtime metrics
- HTTP request metrics

The dashboards are stored in the repository under `grafana/` directories.