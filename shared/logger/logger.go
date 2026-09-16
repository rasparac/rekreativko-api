package logger

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

type (
	Logger struct {
		logger *slog.Logger
	}
)

func New(level, format string) *Logger {
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
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
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
	}
}

func (l *Logger) WithName(name string) *Logger {
	return &Logger{
		logger: l.logger.WithGroup(name),
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

	// Empty (not uuid.Nil.String()) when unset, matching the other fields
	// here - most log lines happen after auth middleware has populated this,
	// but plenty (startup, public endpoints, background jobs) never have it.
	var accountID string
	if id := authcontext.GetAccountID(ctx); id != uuid.Nil {
		accountID = id.String()
	}

	keysAndValues = append(keysAndValues, "ip_address", ipAddress)
	keysAndValues = append(keysAndValues, "user_agent", userAgent)
	keysAndValues = append(keysAndValues, "request_id", requestID)
	keysAndValues = append(keysAndValues, "event_id", eventID)
	keysAndValues = append(keysAndValues, "account_id", accountID)

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip callers, log, and the public method

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])

	r.Add(keysAndValues...)

	_ = l.logger.Handler().Handle(ctx, r)
}
