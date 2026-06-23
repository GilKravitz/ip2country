// Package metrics exposes Prometheus instrumentation for the service.
//
// It owns the collectors and the /metrics scrape handler so the rest of the
// code records observations through a small typed API and never imports the
// prometheus packages directly. Each Metrics uses its own registry, which keeps
// tests isolated (no global state) and avoids duplicate-registration panics.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the service collectors and their registry.
type Metrics struct {
	registry    *prometheus.Registry
	requests    *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	rateLimited prometheus.Counter
}

// New creates and registers the collectors on a fresh registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, route pattern and status code.",
		}, []string{"method", "path", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by method and route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
		rateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "rate_limit_rejections_total",
			Help: "Total requests rejected with HTTP 429 by the rate limiter.",
		}),
	}
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.duration,
		m.rateLimited,
	)
	return m
}

// Handler returns the Prometheus scrape handler for /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveRequest records one completed HTTP request.
func (m *Metrics) ObserveRequest(method, path string, status int, d time.Duration) {
	m.requests.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, path).Observe(d.Seconds())
}

// IncRateLimited records one request rejected by the rate limiter.
func (m *Metrics) IncRateLimited() {
	m.rateLimited.Inc()
}
