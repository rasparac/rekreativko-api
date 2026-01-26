package logger

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
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

	keysAndValues = append(keysAndValues, "ip_address", ipAddress)
	keysAndValues = append(keysAndValues, "user_agent", userAgent)
	keysAndValues = append(keysAndValues, "request_id", requestID)

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip callers, log, and the public method

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])

	r.Add(keysAndValues...)

	_ = l.logger.Handler().Handle(ctx, r)
}
