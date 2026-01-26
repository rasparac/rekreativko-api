package middleware

import (
	"net/http"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

func Tracing(next http.Handler) http.Handler {
	return otelhttp.NewHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			enrichSpan(r)

			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(rw, r)

			ctx := r.Context()

			span := trace.SpanFromContext(ctx)

			if span.IsRecording() {
				span.SetAttributes(
					semconv.HTTPStatusCode(rw.statusCode),
					attribute.Int64("http.response_size", rw.written),
				)

				if rw.statusCode >= http.StatusInternalServerError {
					span.SetAttributes(attribute.Bool("error", true))
				} else if rw.statusCode == http.StatusUnauthorized || rw.statusCode == http.StatusForbidden {
					span.SetAttributes(attribute.Bool("error", true))
					span.SetAttributes(attribute.String("error.type", "security"))
				}
			}
		}),
		"http.server",
		otelhttp.WithSpanNameFormatter(spanNameFormatter),
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)

}

func spanNameFormatter(operation string, r *http.Request) string {
	return r.Method + " " + r.URL.Path
}

func enrichSpan(r *http.Request) {
	ctx := r.Context()

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(
		semconv.HTTPMethod(r.Method),
		semconv.HTTPRoute(r.URL.Path),
		semconv.HTTPScheme(r.URL.Scheme),
		semconv.HTTPTarget(r.URL.RequestURI()),
		semconv.UserAgentOriginal(api.UserAgentFromContext(ctx)),
	)

	ipAddress := api.IpAddressFromContext(ctx)
	if ipAddress != "" {
		span.SetAttributes(attribute.String("http.client_ip", ipAddress))
	}

	requestID := api.RequestIDFromContext(ctx)
	if requestID != "" {
		span.SetAttributes(attribute.String("http.request_id", requestID))
	}
}
