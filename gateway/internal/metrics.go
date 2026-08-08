package gateway

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

	// Notification metrics
	// TOOD: move this to metrics service!!!
	SendTotal    *prometheus.CounterVec
	SendFailures *prometheus.CounterVec
	SendDuration *prometheus.HistogramVec
}

func New(namespace string) *Metrics {
	constLabels := prometheus.Labels{"namespace": namespace}

	return &Metrics{
		HTTPRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "http_requests_total",
				Help:        "Total number of HTTP requests",
				ConstLabels: constLabels,
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   namespace,
				Name:        "http_request_duration_seconds",
				Help:        "Duration of HTTP requests in seconds",
				Buckets:     prometheus.DefBuckets,
				ConstLabels: constLabels,
			},
			[]string{"method", "path", "status"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   namespace,
				Name:        "http_response_size_bytes",
				Help:        "Size of HTTP responses in bytes",
				Buckets:     prometheus.ExponentialBuckets(100, 10, 7),
				ConstLabels: constLabels,
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   namespace,
				Name:        "http_request_size_bytes",
				Help:        "Size of HTTP requests in bytes",
				Buckets:     prometheus.ExponentialBuckets(100, 10, 7),
				ConstLabels: constLabels,
			},
			[]string{"method", "path"},
		),

		// Notifications

		SendTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   "notifications",
				Name:        "send_total",
				Help:        "Total number of notification sends",
				ConstLabels: constLabels,
			},
			[]string{"type", "channel"},
		),
		SendDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   namespace,
				Subsystem:   "notifications",
				Name:        "send_duration_seconds",
				Help:        "Duration of notification sends in seconds",
				Buckets:     prometheus.DefBuckets,
				ConstLabels: constLabels,
			},
			[]string{"type", "channel"},
		),
		SendFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   "notifications",
				Name:        "send_failures_total",
				Help:        "Total number of notification send failures",
				ConstLabels: constLabels,
			},
			[]string{"type", "channel", "reason"},
		),
	}
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
