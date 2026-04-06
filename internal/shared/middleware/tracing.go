package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

func Tracing(next http.Handler) http.Handler {

	return otelhttp.NewHandler(
		next,
		"http.server",
		otelhttp.WithSpanNameFormatter(spanNameFormatter),
		otelhttp.WithSpanOptions(trace.WithSpanKind(trace.SpanKindServer)),
		otelhttp.WithFilter(func(r *http.Request) bool {
			return !strings.Contains(r.URL.Path, "/health")
		}),
	)

	// return otelhttp.NewHandler(
	// 	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

	// 		ctx := r.Context()

	// 		ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))

	// 		enrichSpan(ctx, r)

	// 		rw := &responseWriter{
	// 			ResponseWriter: w,
	// 			statusCode:     http.StatusOK,
	// 		}

	// 		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(w.Header()))

	// 		next.ServeHTTP(rw, r)

	// 		span := trace.SpanFromContext(ctx)

	// 		if span.IsRecording() {
	// 			span.SetAttributes(
	// 				semconv.HTTPStatusCode(rw.statusCode),
	// 				attribute.Int64("http.response_size", rw.written),
	// 			)

	// 			if rw.statusCode >= http.StatusInternalServerError {
	// 				span.SetAttributes(attribute.Bool("error", true))
	// 			} else if rw.statusCode == http.StatusUnauthorized || rw.statusCode == http.StatusForbidden {
	// 				span.SetAttributes(attribute.Bool("error", true))
	// 				span.SetAttributes(attribute.String("error.type", "security"))
	// 			}
	// 		}
	// 	}),
	// 	"http.server",
	// 	otelhttp.WithSpanNameFormatter(spanNameFormatter),
	// 	otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	// )

}

func SpanEnrichment(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		span := enrichSpan(ctx, r)

		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rw, r)

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
	})
}

func spanNameFormatter(operation string, r *http.Request) string {
	return r.Method + " " + r.URL.Path
}

func enrichSpan(ctx context.Context, r *http.Request) trace.Span {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return span
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

	return span
}
