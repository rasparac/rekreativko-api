package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

type testHTTPMetrics struct {
	requestSize, requestDuration, responseSize *prometheus.HistogramVec
	requestTotal                               *prometheus.CounterVec
}

func newTestHTTPMetrics() *testHTTPMetrics {
	labels := []string{"method", "path", "status"}
	return &testHTTPMetrics{
		requestSize:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "request_size"}, []string{"method", "path"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "request_duration"}, labels),
		responseSize:    prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "response_size"}, labels),
		requestTotal:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "request_total"}, labels),
	}
}

func (m *testHTTPMetrics) RequestSize() *prometheus.HistogramVec     { return m.requestSize }
func (m *testHTTPMetrics) RequestTotal() *prometheus.CounterVec      { return m.requestTotal }
func (m *testHTTPMetrics) RequestDuration() *prometheus.HistogramVec { return m.requestDuration }
func (m *testHTTPMetrics) ResponseSize() *prometheus.HistogramVec    { return m.responseSize }

// countSeries returns how many series the collector currently holds.
func countSeries(c prometheus.Collector) int {
	ch := make(chan prometheus.Metric, 16)
	go func() {
		c.Collect(ch)
		close(ch)
	}()

	n := 0
	for range ch {
		n++
	}
	return n
}

func TestMetrics_EventStreamsStayOutOfTheLatencyHistograms(t *testing.T) {
	tests := []struct {
		name         string
		contentType  string
		wantObserved int
	}{
		{"ordinary response is observed", "application/json", 1},
		{"event stream is not", "text/event-stream", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestHTTPMetrics()
			handler := Metrics(m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				_, _ = w.Write([]byte("data"))
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

			assert.Equal(t, 1, countSeries(m.requestTotal), "every request is counted")
			assert.Equal(t, tt.wantObserved, countSeries(m.requestDuration))
			assert.Equal(t, tt.wantObserved, countSeries(m.responseSize))
		})
	}
}
