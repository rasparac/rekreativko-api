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
}

func New() *Metrics {
	return &Metrics{
		// Event
		EventsPublishedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "events",
				Name:      "published_total",
				Help:      "Total number of events published",
			},
			[]string{"event_type", "schema"},
		),
		EventPublishDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: "events",
				Name:      "publish_duration_seconds",
				Help:      "Duration of event publishing in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"event_type", "schema"},
		),
		EventProcessedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "events",
				Name:      "processed_total",
				Help:      "Total number of events processed",
			},
			[]string{"event_type", "schema", "status"},
		),

		// Database query
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
	}
}

func (m *Metrics) DatabaseQueryDuration() *prometheus.HistogramVec {
	return m.DBQueryDuration
}

func (m *Metrics) DatabaseQueryTotal() *prometheus.CounterVec {
	return m.DBQueryTotal
}
