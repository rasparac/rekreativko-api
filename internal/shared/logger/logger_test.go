package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
