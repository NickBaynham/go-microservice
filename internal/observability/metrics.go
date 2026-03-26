package observability

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	metricsNS = "gomicro"
	metricsSS = "http"
)

// Prometheus HTTP metrics (RED-oriented). Route label uses the matched pattern (e.g. /users/:id) to
// keep cardinality bounded; unmatched routes are recorded as "(unmatched)".
var (
	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNS,
		Subsystem: metricsSS,
		Name:      "in_flight_requests",
		Help:      "Current number of HTTP requests being served.",
	})

	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNS,
		Subsystem: metricsSS,
		Name:      "requests_total",
		Help:      "Total HTTP requests by method, route pattern, and status code.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNS,
		Subsystem: metricsSS,
		Name:      "request_duration_seconds",
		Help:      "HTTP request latencies in seconds by method and route pattern.",
		Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})
)

func routeLabel(c *gin.Context) string {
	p := c.FullPath()
	if p == "" {
		return "(unmatched)"
	}
	return p
}

// PrometheusMiddleware records in-flight gauge, request counts, and latency histograms.
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		start := time.Now()
		c.Next()

		route := routeLabel(c)
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())
		dur := time.Since(start).Seconds()

		httpRequests.WithLabelValues(method, route, status).Inc()
		httpDuration.WithLabelValues(method, route).Observe(dur)
	}
}
