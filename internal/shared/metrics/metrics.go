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
	IdentityRegistrationsTotal    *prometheus.CounterVec
	IdentityLoginAttemptsTotal    *prometheus.CounterVec
	IdentityLoginSuccessTotal     prometheus.Counter
	IdentityLoginFailuresTotal    *prometheus.CounterVec
	IdentityVerificationTotal     *prometheus.CounterVec
	IdentityActiveRefreshesTokens prometheus.Gauge

	// Event metrics
	EventsPublishedTotal *prometheus.CounterVec
	EventPublishDuration *prometheus.HistogramVec
	EventProcessedTotal  *prometheus.CounterVec

	// Database metrics
	DatabaseQueryTotal    *prometheus.CounterVec
	DatabaseQueryDuration *prometheus.HistogramVec

	// Notification metrics
	NotificationSendTotal    *prometheus.CounterVec
	NotificationSendFailures *prometheus.CounterVec
	NotificationSendDuration *prometheus.HistogramVec
}

func New(namespace string) *Metrics {
	return &Metrics{
		HTTPRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "Duration of HTTP requests in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "Size of HTTP responses in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 7),
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_size_bytes",
				Help:      "Size of HTTP requests in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 7),
			},
			[]string{"method", "path"},
		),

		// Identity
		IdentityRegistrationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "identity",
				Name:      "registrations_total",
				Help:      "Total number of user registrations",
			},
			[]string{"method"}, // email or phone
		),
		IdentityLoginAttemptsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "identity",
				Name:      "login_attempts_total",
				Help:      "Total number of login attempts",
			},
			[]string{"method"},
		),
		IdentityLoginSuccessTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "identity",
				Name:      "login_success_total",
				Help:      "Total number of login successes",
			},
		),
		IdentityLoginFailuresTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "identity",
				Name:      "login_failures_total",
				Help:      "Total number of login failures",
			},
			[]string{"reason"},
		),
		IdentityVerificationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "identity",
				Name:      "verification_total",
				Help:      "Total number of verifications",
			},
			[]string{"status"},
		),
		IdentityActiveRefreshesTokens: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "identity_active_refreshes_tokens",
				Help:      "Current number of active refreshes tokens",
			},
		),

		// Event
		EventsPublishedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "events",
				Name:      "published_total",
				Help:      "Total number of events published",
			},
			[]string{"event_type"},
		),
		EventPublishDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "events",
				Name:      "publish_duration_seconds",
				Help:      "Duration of event publishing in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"event_type"},
		),
		EventProcessedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "events",
				Name:      "processed_total",
				Help:      "Total number of events processed",
			},
			[]string{"event_type", "status"},
		),

		// Database query
		DatabaseQueryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "query_total",
				Help:      "Total number of database queries",
			},
			[]string{"operation", "status", "table"},
		),
		DatabaseQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "query_duration_seconds",
				Help:      "Duration of database queries in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation", "status", "table"},
		),

		// Notifications

		NotificationSendTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "notifications",
				Name:      "send_total",
				Help:      "Total number of notification sends",
			},
			[]string{"type", "channel"},
		),
		NotificationSendDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "notifications",
				Name:      "send_duration_seconds",
				Help:      "Duration of notification sends in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"type", "channel"},
		),
		NotificationSendFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "notifications",
				Name:      "send_failures_total",
				Help:      "Total number of notification send failures",
			},
			[]string{"type", "channel", "reason"},
		),
	}
}
