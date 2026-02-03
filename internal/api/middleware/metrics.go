package middleware

import (
	"net/http"
	"sync/atomic"
)

// MetricsCollector collects request metrics.
type MetricsCollector struct {
	requestCount *atomic.Int64
	errorCount   *atomic.Int64
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(requestCount, errorCount *atomic.Int64) *MetricsCollector {
	return &MetricsCollector{
		requestCount: requestCount,
		errorCount:   errorCount,
	}
}

// Middleware returns middleware that counts requests and errors.
func (mc *MetricsCollector) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mc.requestCount.Add(1)

		// Wrap response writer to capture status
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		// Count errors (4xx and 5xx)
		if rw.statusCode >= 400 {
			mc.errorCount.Add(1)
		}
	})
}
