package events

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
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
	return &Metrics{
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

func (m *Metrics) DatabaseQueryDuration() *prometheus.HistogramVec {
	return m.DBQueryDuration
}

func (m *Metrics) DatabaseQueryTotal() *prometheus.CounterVec {
	return m.DBQueryTotal
}
