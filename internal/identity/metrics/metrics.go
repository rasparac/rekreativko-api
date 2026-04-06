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

	// Business metrics
	RegistrationsTotal    *prometheus.CounterVec
	LoginAttemptsTotal    *prometheus.CounterVec
	LoginSuccessTotal     prometheus.Counter
	LoginFailuresTotal    *prometheus.CounterVec
	VerificationTotal     *prometheus.CounterVec
	ActiveRefreshesTokens prometheus.Gauge

	// Event metrics
	EventsPublishedTotal *prometheus.CounterVec
	EventPublishDuration *prometheus.HistogramVec
	EventProcessedTotal  *prometheus.CounterVec

	// Database metrics
	DBQueryTotal    *prometheus.CounterVec
	DBQueryDuration *prometheus.HistogramVec

	// Notification metrics
	NotificationSendTotal    *prometheus.CounterVec
	NotificationSendFailures *prometheus.CounterVec
	NotificationSendDuration *prometheus.HistogramVec
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

		RegistrationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "identity",
				Name:      "registrations_total",
				Help:      "Total number of account registrations",
			},
			[]string{"method"}, // email or phone
		),
		LoginAttemptsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "login_attempts_total",
				Help:      "Total number of login attempts",
			},
			[]string{"method"},
		),
		LoginSuccessTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "login_success_total",
				Help:        "Total number of login successes",
				ConstLabels: constLabels,
			},
		),
		LoginFailuresTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "login_failures_total",
				Help:        "Total number of login failures",
				ConstLabels: constLabels,
			},
			[]string{"reason"},
		),
		VerificationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Name:        "verification_total",
				Help:        "Total number of verifications",
				ConstLabels: constLabels,
			},
			[]string{"status"},
		),
		ActiveRefreshesTokens: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace:   namespace,
				Name:        "refreshes_tokens",
				Help:        "Current number of active refreshes tokens",
				ConstLabels: constLabels,
			},
		),

		// Event
		EventsPublishedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   "events",
				Name:        "published_total",
				Help:        "Total number of events published",
				ConstLabels: constLabels,
			},
			[]string{"event_type"},
		),
		EventPublishDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   namespace,
				Subsystem:   "events",
				Name:        "publish_duration_seconds",
				Help:        "Duration of event publishing in seconds",
				Buckets:     prometheus.DefBuckets,
				ConstLabels: constLabels,
			},
			[]string{"event_type"},
		),
		EventProcessedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   "events",
				Name:        "processed_total",
				Help:        "Total number of events processed",
				ConstLabels: constLabels,
			},
			[]string{"event_type", "status"},
		),

		// Database query
		DBQueryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   "database",
				Name:        "query_total",
				Help:        "Total number of database queries",
				ConstLabels: constLabels,
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

		// Notifications

		NotificationSendTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   "notifications",
				Name:        "send_total",
				Help:        "Total number of notification sends",
				ConstLabels: constLabels,
			},
			[]string{"type", "channel"},
		),
		NotificationSendDuration: promauto.NewHistogramVec(
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
		NotificationSendFailures: promauto.NewCounterVec(
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
