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

	// Geocode cache metrics
	CacheHits   prometheus.Counter
	CacheMisses prometheus.Counter

	// Forward-geocode cache metrics
	ForwardCacheHits   prometheus.Counter
	ForwardCacheMisses prometheus.Counter

	// Outbound Nominatim call metrics
	NominatimCallTotal    prometheus.Counter
	NominatimCallDuration prometheus.Histogram
	NominatimCallFailures *prometheus.CounterVec
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

		CacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "geocode",
				Name:      "cache_hits_total",
				Help:      "Total number of reverse-geocode lookups served from the cache",
			},
		),
		CacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "geocode",
				Name:      "cache_misses_total",
				Help:      "Total number of reverse-geocode lookups that required a Nominatim call",
			},
		),

		ForwardCacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "geocode",
				Name:      "forward_cache_hits_total",
				Help:      "Total number of forward-geocode lookups served from the cache",
			},
		),
		ForwardCacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "geocode",
				Name:      "forward_cache_misses_total",
				Help:      "Total number of forward-geocode lookups that required a Nominatim call",
			},
		),

		NominatimCallTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Subsystem: "nominatim",
				Name:      "calls_total",
				Help:      "Total number of outbound calls to Nominatim",
			},
		),
		NominatimCallDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Subsystem: "nominatim",
				Name:      "call_duration_seconds",
				Help:      "Duration of outbound Nominatim calls in seconds",
				Buckets:   prometheus.DefBuckets,
			},
		),
		NominatimCallFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: "nominatim",
				Name:      "call_failures_total",
				Help:      "Total number of failed outbound Nominatim calls",
			},
			[]string{"reason"},
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
