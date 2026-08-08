package middleware

import (
	"net/http"
	"strconv"
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

			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

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
