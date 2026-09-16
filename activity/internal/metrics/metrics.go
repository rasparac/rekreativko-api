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

	// Business metrics - Activity Groups
	ActivityGroupCreatedTotal prometheus.Counter
	ActivityGroupUpdatedTotal prometheus.Counter
	ActivityGroupDeletedTotal prometheus.Counter

	// Business metrics - Sessions
	SessionCreatedTotal prometheus.Counter
	SessionUpdatedTotal prometheus.Counter
	SessionDeletedTotal prometheus.Counter

	// Business metrics - Session Templates
	SessionTemplateCreatedTotal prometheus.Counter
	SessionTemplateUpdatedTotal prometheus.Counter
	SessionTemplateDeletedTotal prometheus.Counter

	// Business metrics - Members
	MemberJoinedTotal   prometheus.Counter
	MemberLeftTotal     prometheus.Counter
	MemberInvitedTotal  prometheus.Counter
	MemberAcceptedTotal prometheus.Counter

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
		HTTPRequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_request_duration_seconds",
				Help: "HTTP request duration in seconds",
			},
			[]string{"method", "path", "status"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_response_size_bytes",
				Help: "HTTP response size in bytes",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "http_request_size_bytes",
				Help: "HTTP request size in bytes",
			},
			[]string{"method", "path"},
		),
		ActivityGroupCreatedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "activity_group_created_total",
				Help: "Total number of activity groups created",
			},
		),
		ActivityGroupUpdatedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "activity_group_updated_total",
				Help: "Total number of activity groups updated",
			},
		),
		ActivityGroupDeletedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "activity_group_deleted_total",
				Help: "Total number of activity groups deleted",
			},
		),
		SessionCreatedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "session_created_total",
				Help: "Total number of sessions created",
			},
		),
		SessionUpdatedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "session_updated_total",
				Help: "Total number of sessions updated",
			},
		),
		SessionDeletedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "session_deleted_total",
				Help: "Total number of sessions deleted",
			},
		),
		SessionTemplateCreatedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "session_template_created_total",
				Help: "Total number of session templates created",
			},
		),
		SessionTemplateUpdatedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "session_template_updated_total",
				Help: "Total number of session templates updated",
			},
		),
		SessionTemplateDeletedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "session_template_deleted_total",
				Help: "Total number of session templates deleted",
			},
		),
		MemberJoinedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "member_joined_total",
				Help: "Total number of members joined",
			},
		),
		MemberLeftTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "member_left_total",
				Help: "Total number of members left",
			},
		),
		MemberInvitedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "member_invited_total",
				Help: "Total number of members invited",
			},
		),
		MemberAcceptedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "member_accepted_total",
				Help: "Total number of member invitations accepted",
			},
		),
		EventsPublishedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "events",
				Name:      "published_total",
				Help:      "Total number of domain events published",
			},
			[]string{"event_type"},
		),
		EventPublishDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: "events",
				Name:      "publish_duration_seconds",
				Help:      "Event publish duration in seconds",
			},
			[]string{"event_type"},
		),
		EventProcessedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "events",
				Name:      "processed_total",
				Help:      "Total number of domain events processed",
			},
			[]string{"event_type", "status"},
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
				Help:      "Database query duration in seconds",
			},
			[]string{"operation", "status", "table"},
		),
	}
}

// DatabaseQueryDuration returns the database query duration histogram
func (m *Metrics) DatabaseQueryDuration() *prometheus.HistogramVec {
	return m.DBQueryDuration
}

// DatabaseQueryTotal returns the database query total counter
func (m *Metrics) DatabaseQueryTotal() *prometheus.CounterVec {
	return m.DBQueryTotal
}

// RequestSize returns the HTTP request size histogram
func (m *Metrics) RequestSize() *prometheus.HistogramVec {
	return m.HTTPRequestSize
}

// RequestTotal returns the HTTP request total counter
func (m *Metrics) RequestTotal() *prometheus.CounterVec {
	return m.HTTPRequestTotal
}

// RequestDuration returns the HTTP request duration histogram
func (m *Metrics) RequestDuration() *prometheus.HistogramVec {
	return m.HTTPRequestDuration
}

// ResponseSize returns the HTTP response size histogram
func (m *Metrics) ResponseSize() *prometheus.HistogramVec {
	return m.HTTPResponseSize
}
