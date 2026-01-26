package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/metrics"
)

func Metrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			if r.ContentLength > 0 {
				m.HTTPRequestSize.WithLabelValues(
					r.Method,
					r.URL.Path,
				).Observe(float64(r.ContentLength))
			}

			next.ServeHTTP(rw, r)

			status := strconv.Itoa(rw.statusCode)

			// Request count
			m.HTTPRequestTotal.WithLabelValues(
				r.Method,
				r.URL.Path,
				status,
			).Inc()

			duration := time.Since(start).Seconds()

			// Request duration
			m.HTTPRequestDuration.WithLabelValues(
				r.Method,
				r.URL.Path,
				status,
			).Observe(duration)

			// Response size
			m.HTTPResponseSize.WithLabelValues(
				r.Method,
				r.URL.Path,
				status,
			).Observe(float64(rw.written))

		})
	}
}
