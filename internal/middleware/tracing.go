package middleware

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TraceIDKey is the context key under which the active trace ID string is
// stored for correlation with logging and the request-id path.
const TraceIDKey contextKey = "trace_id"

// SpanIDKey is the context key under which the active span ID string is stored.
const SpanIDKey contextKey = "span_id"

// Tracing returns a middleware that starts a server span per request using the
// supplied tracer. It extracts inbound W3C trace context (traceparent/
// tracestate), names the span by the normalized mux route template (to avoid
// high-cardinality span names), records method/route/status attributes, and
// stores the trace_id/span_id into the request context for log correlation.
//
// When the provided tracer comes from a no-op TracerProvider (tracing
// disabled), span creation is effectively free and no spans are exported, so
// this middleware is safe to install unconditionally.
func Tracing(tracer trace.Tracer, propagator propagation.TextMapPropagator) Middleware {
	if propagator == nil {
		propagator = propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract any inbound trace context so the new span links to the
			// caller's trace (distributed tracing).
			ctx := propagator.Extract(
				r.Context(), propagation.HeaderCarrier(r.Header),
			)

			route := normalizeRequestPath(r)
			spanName := r.Method + " " + route

			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("http.route", route),
					attribute.String("url.path", r.URL.Path),
					attribute.String("server.address", r.Host),
					attribute.String("user_agent.original", r.UserAgent()),
					attribute.String("request.id", getRequestID(r)),
				),
			)
			defer span.End()

			// Correlate trace identifiers into the context so the Logging
			// middleware (and handlers) can emit trace_id/span_id fields.
			sc := span.SpanContext()
			if sc.HasTraceID() {
				ctx = context.WithValue(ctx, TraceIDKey, sc.TraceID().String())
				ctx = context.WithValue(ctx, SpanIDKey, sc.SpanID().String())
			}

			rw := newResponseWriter(w)
			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetAttributes(attribute.Int("http.response.status_code", rw.statusCode))
			if rw.statusCode >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(rw.statusCode))
			}
		})
	}
}

// TraceIDFromContext extracts the correlated trace ID string from the context,
// returning empty string when no traced request is active.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(TraceIDKey).(string); ok {
		return v
	}
	return ""
}

// SpanIDFromContext extracts the correlated span ID string from the context,
// returning empty string when no traced request is active.
func SpanIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(SpanIDKey).(string); ok {
		return v
	}
	return ""
}
