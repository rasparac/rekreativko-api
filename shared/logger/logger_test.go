package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each context field appears once per line: a key the caller passes wins,
// otherwise the context's value is used.
func Test_logger_contextFields(t *testing.T) {
	ctx := api.WithEventID(context.Background(), "from-ctx")

	for name, tc := range map[string]struct {
		log  func(l *Logger)
		want string
	}{
		"from the context": {
			log:  func(l *Logger) { l.Info(ctx, "msg") },
			want: "from-ctx",
		},
		"passed as key and value": {
			log:  func(l *Logger) { l.Info(context.Background(), "msg", "event_id", "explicit") },
			want: "explicit",
		},
		"passed as an slog.Attr": {
			log:  func(l *Logger) { l.Info(ctx, "msg", slog.String("event_id", "attr"), "other", 1) },
			want: "attr",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			tc.log(NewWithWriter("debug", "json", &buf))

			line := buf.String()
			assert.Equal(t, 1, strings.Count(line, `"event_id"`), line)

			var got map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
			assert.Equal(t, tc.want, got["event_id"])
		})
	}
}

func Test_logger_New(t *testing.T) {
	testCases := []struct {
		name   string
		level  string
		format string
	}{
		{
			name:   "info level with text format",
			level:  "info",
			format: "text",
		},
		{
			name:   "debug level with json format",
			level:  "debug",
			format: "json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotLogger := New(tc.level, tc.format)

			assert.NotNil(t, gotLogger)
		})
	}
}
