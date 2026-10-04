package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type (
	httpMetrics interface {
		RequestSize() *prometheus.HistogramVec
		RequestTotal() *prometheus.CounterVec
		RequestDuration() *prometheus.HistogramVec
		ResponseSize() *prometheus.HistogramVec
	}
)

func Metrics(m httpMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := newResponseWriter(w)

			if r.ContentLength > 0 {
				m.RequestSize().WithLabelValues(
					r.Method,
					r.URL.Path,
				).Observe(float64(r.ContentLength))
			}

			next.ServeHTTP(rw, r)

			status := strconv.Itoa(rw.statusCode)

			m.RequestTotal().WithLabelValues(
				r.Method,
				r.URL.Path,
				status,
			).Inc()

			// A server-sent-events stream stays open for minutes: its duration
			// and size would only skew the latency histograms. It is still
			// counted above.
			if strings.HasPrefix(rw.Header().Get("Content-Type"), "text/event-stream") {
				return
			}

			duration := time.Since(start).Seconds()

			m.RequestDuration().WithLabelValues(
				r.Method,
				r.URL.Path,
				status,
			).Observe(duration)

			m.ResponseSize().WithLabelValues(
				r.Method,
				r.URL.Path,
				status,
			).Observe(float64(rw.written))
		})
	}
}
