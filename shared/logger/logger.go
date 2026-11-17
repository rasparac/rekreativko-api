package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

type (
	Logger struct {
		logger *slog.Logger
		// name is written as one top-level "logger" field, not as a slog
		// group: a group nests every key under it, and Loki's | json then
		// names a field per component (activity_http_moderation_event_id).
		name string
	}
)

func New(level, format string) *Logger {
	return NewWithWriter(level, format, os.Stdout)
}

// NewWithWriter is New writing to w instead of stdout (tests read what was
// logged this way).
func NewWithWriter(level, format string, w io.Writer) *Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	replace := func(groups []string, a slog.Attr) slog.Attr {

		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.Attr{}
		}

		if a.Key == slog.SourceKey {
			source, ok := a.Value.Any().(*slog.Source)
			if !ok {
				return a
			}
			source.File = filepath.Base(source.File)
		}
		return a
	}

	opts := &slog.HandlerOptions{
		Level:       logLevel,
		AddSource:   true,
		ReplaceAttr: replace,
	}

	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	return &Logger{
		logger: slog.New(handler),
	}
}

func NewDevelopment() *Logger {
	return New("debug", "text")
}

func (l *Logger) WithValues(values ...any) *Logger {
	return &Logger{
		logger: l.logger.With(values...),
		name:   l.name,
	}
}

// WithName appends name to the logger's name with a dot. A name that already
// starts with it replaces it: main names the logger after the service and
// components usually spell the full "<service>.<component>".
func (l *Logger) WithName(name string) *Logger {
	switch {
	case l.name == "", name == l.name, strings.HasPrefix(name, l.name+"."):
	default:
		name = l.name + "." + name
	}

	return &Logger{
		logger: l.logger,
		name:   name,
	}
}

func (l *Logger) Debug(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, slog.LevelDebug, msg, keysAndValues...)
}

func (l *Logger) Info(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, slog.LevelInfo, msg, keysAndValues...)
}

func (l *Logger) Warn(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, slog.LevelWarn, msg, keysAndValues...)
}

func (l *Logger) Error(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, slog.LevelError, msg, keysAndValues...)
}

func (l *Logger) log(
	ctx context.Context,
	level slog.Level,
	msg string,
	keysAndValues ...any,
) {
	if !l.logger.Enabled(ctx, level) {
		return
	}

	requestID := api.RequestIDFromContext(ctx)
	ipAddress := api.IpAddressFromContext(ctx)
	userAgent := api.UserAgentFromContext(ctx)
	eventID := api.EventIDFromContext(ctx)

	// Left out (not uuid.Nil.String()) when unset: startup, public endpoints
	// and background jobs never have it.
	var accountID string
	if id := authcontext.GetAccountID(ctx); id != uuid.Nil {
		accountID = id.String()
	}

	// A key the caller passed wins over the context's value: the JSON
	// handler doesn't dedupe, and a second "event_id" would hide the real one
	// from anything that reads the line as JSON (jq, Loki). An unset value is
	// left out rather than logged empty.
	for _, field := range [...]struct{ key, value string }{
		{"logger", l.name},
		{"ip_address", ipAddress},
		{"user_agent", userAgent},
		{"request_id", requestID},
		{"event_id", eventID},
		{"account_id", accountID},
	} {
		if field.value != "" && !hasKey(keysAndValues, field.key) {
			keysAndValues = append(keysAndValues, field.key, field.value)
		}
	}

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip callers, log, and the public method

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])

	r.Add(keysAndValues...)

	_ = l.logger.Handler().Handle(ctx, r)
}

// hasKey reports whether the key-value list sets key. It walks the list the
// way slog.Record.Add does: an slog.Attr takes one slot, a key and its value two.
func hasKey(keysAndValues []any, key string) bool {
	for i := 0; i < len(keysAndValues); {
		switch k := keysAndValues[i].(type) {
		case slog.Attr:
			if k.Key == key {
				return true
			}
			i++
		case string:
			if k == key {
				return true
			}
			i += 2
		default:
			i++
		}
	}
	return false
}
