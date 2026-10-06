package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

func TestRedactQuery(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"nothing", "", ""},
		{"allowed keys keep their values, sorted", "status=going&limit=20&activity_type=basketball", "activity_type=basketball&limit=20&status=going"},
		{"ids are kept", "session_id=0b6e6f4e-1a2b-4c3d-8e9f-0a1b2c3d4e5f", "session_id=0b6e6f4e-1a2b-4c3d-8e9f-0a1b2c3d4e5f"},
		{"secrets are redacted but the key stays", "token=abc123&code=999999&password=hunter2", "code=[redacted]&password=[redacted]&token=[redacted]"},
		{"personal data is redacted", "lat=44.8&lon=20.4&lng=20.4&street=Knez+Mihailova&dob_gt=1990-01-01&title=my+name", "dob_gt=[redacted]&lat=[redacted]&lng=[redacted]&lon=[redacted]&street=[redacted]&title=[redacted]"},
		{"paging cursors are redacted", "page_token=eyJzb3J0Ijoi", "page_token=[redacted]"},
		{"a parameter nobody listed is redacted by default", "magic_link=s3cr3t", "magic_link=[redacted]"},
		{"allowed and redacted mixed", "status=going&token=abc&limit=5", "limit=5&status=going&token=[redacted]"},
		{"repeated keys are all handled", "status=going&status=maybe&token=a&token=b", "status=going&status=maybe&token=[redacted]&token=[redacted]"},
		{"values are escaped", "city=Novi+Sad", "city=Novi+Sad"},
		{"a key can't smuggle a value through", "token%3Dabc=1", "token%3Dabc=[redacted]"},
		{"an unparseable query is not logged at all", "status=%zz&token=abc", "[unparseable]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactQuery(tt.raw)

			assert.Equal(t, tt.want, got)
			for _, secret := range []string{"abc123", "hunter2", "999999", "s3cr3t", "Mihailova", "44.8", "1990-01-01"} {
				assert.NotContains(t, got, secret)
			}
		})
	}
}

// What the access log really writes - not just what the helper returns.
func TestLogging_NeverWritesSensitiveQueryValues(t *testing.T) {
	var out bytes.Buffer
	log := logger.NewWithWriter("info", "json", &out)

	handler := Logging(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet,
		"/api/v1/sessions?status=going&lat=44.8123&lon=20.4567&token=s3cr3t-token&limit=10", nil))

	var line map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &line), out.String())
	assert.Equal(t, "lat=[redacted]&limit=10&lon=[redacted]&status=going&token=[redacted]", line["query_params"])
	assert.Equal(t, "/api/v1/sessions", line["path"])
	for _, secret := range []string{"s3cr3t-token", "44.8123", "20.4567"} {
		assert.NotContains(t, out.String(), secret, "the log line must not contain it anywhere")
	}
}

type spanCollector struct{ spans []sdktrace.ReadOnlySpan }

func (c *spanCollector) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	c.spans = append(c.spans, spans...)
	return nil
}

func (c *spanCollector) Shutdown(context.Context) error { return nil }

// The trace's http.target carried the whole query string too.
func TestEnrichSpan_NeverRecordsSensitiveQueryValues(t *testing.T) {
	collector := &spanCollector{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))

	req := httptest.NewRequest(http.MethodGet, "/location/api/v1/search?city=Novi+Sad&lat=44.8123&token=s3cr3t-token", nil)
	ctx, span := provider.Tracer("test").Start(req.Context(), "request")
	enrichSpan(ctx, req)
	span.End()

	require.Len(t, collector.spans, 1)
	var target string
	for _, attr := range collector.spans[0].Attributes() {
		if attr.Key == attribute.Key("http.target") {
			target = attr.Value.AsString()
		}
	}

	assert.Equal(t, "/location/api/v1/search?city=Novi+Sad&lat=[redacted]&token=[redacted]", target)
	assert.NotContains(t, target, "s3cr3t-token")
	assert.NotContains(t, target, "44.8123")
}
