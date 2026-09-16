package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	// HTTP metrics
	HTTPRequestTotal    *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec

	// Database metrics
	DBQueryTotal    *prometheus.CounterVec
	DBQueryDuration *prometheus.HistogramVec

	// Notification metrics
	NotificationCreated prometheus.Counter
	NotificationRead    prometheus.Counter

	// Notification send metrics (email/SMS delivery, moved here from gateway -
	// this service now owns every way of notifying a user, not just the
	// in-app feed)
	SendTotal    *prometheus.CounterVec
	SendFailures *prometheus.CounterVec
	SendDuration *prometheus.HistogramVec
}

func New() *Metrics {
	return &Metrics{
		HTTPRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "Size of HTTP responses in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 7),
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_size_bytes",
				Help:    "Size of HTTP requests in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 7),
			},
			[]string{"method", "path"},
		),

		DBQueryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "database",
				Name:      "query_total",
				Help:      "Total number of database queries",
			},
			[]string{"operation", "status", "table"},
		),
		DBQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: "database",
				Name:      "query_duration_seconds",
				Help:      "Duration of database queries in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation", "status", "table"},
		),

		NotificationCreated: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "created_total",
				Help: "Total number of notifications created",
			},
		),
		NotificationRead: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "read_total",
				Help: "Total number of notifications marked read",
			},
		),

		SendTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "notifications",
				Name:      "send_total",
				Help:      "Total number of notification sends",
			},
			[]string{"type", "channel"},
		),
		SendDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: "notifications",
				Name:      "send_duration_seconds",
				Help:      "Duration of notification sends in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"type", "channel"},
		),
		SendFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "notifications",
				Name:      "send_failures_total",
				Help:      "Total number of notification send failures",
			},
			[]string{"type", "channel", "reason"},
		),
	}
}

func (m *Metrics) DatabaseQueryDuration() *prometheus.HistogramVec {
	return m.DBQueryDuration
}

func (m *Metrics) DatabaseQueryTotal() *prometheus.CounterVec {
	return m.DBQueryTotal
}

func (m *Metrics) RequestSize() *prometheus.HistogramVec {
	return m.HTTPRequestSize
}

func (m *Metrics) RequestTotal() *prometheus.CounterVec {
	return m.HTTPRequestTotal
}

func (m *Metrics) RequestDuration() *prometheus.HistogramVec {
	return m.HTTPRequestDuration
}

func (m *Metrics) ResponseSize() *prometheus.HistogramVec {
	return m.HTTPResponseSize
}

func (m *Metrics) NotificationSendTotal() *prometheus.CounterVec {
	return m.SendTotal
}

func (m *Metrics) NotificationSendFailures() *prometheus.CounterVec {
	return m.SendFailures
}

func (m *Metrics) NotificationSendDuration() *prometheus.HistogramVec {
	return m.SendDuration
}
