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

	// Account Profile metrics
	ProfileCreated prometheus.Counter
	ProfileDeleted prometheus.Counter
	ProfileUpdated prometheus.Counter

	// Settings metrics
	SettingsUpdated *prometheus.CounterVec
	SettingsReset   prometheus.Counter
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

		// Database query
		DBQueryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "query_total",
				Help:      "Total number of database queries",
			},
			[]string{"operation", "status", "table"},
		),
		DBQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   namespace,
				Subsystem:   "database",
				Name:        "query_duration_seconds",
				Help:        "Duration of database queries in seconds",
				Buckets:     prometheus.DefBuckets,
				ConstLabels: constLabels,
			},
			[]string{"operation", "status", "table"},
		),

		// Account Profile
		ProfileCreated: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "created_total",
				Help:      "Total number of account profiles created",
			},
		),
		ProfileDeleted: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "deleted_total",
				Help:        "Total number of account profiles deleted",
				ConstLabels: constLabels,
			},
		),
		ProfileUpdated: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "updated_total",
				Help:        "Total number of account profiles updated",
				ConstLabels: constLabels,
			},
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
