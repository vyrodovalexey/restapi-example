package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// newRecordingTracer returns an SDK tracer backed by an in-memory span
// recorder so tests can assert on emitted spans deterministically.
func newRecordingTracer() (trace.Tracer, *tracetest.SpanRecorder) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	return tp.Tracer("test"), sr
}

func TestTracing_CreatesSpanWithAttributes(t *testing.T) {
	// Arrange
	tracer, sr := newRecordingTracer()

	var ctxTraceID, ctxSpanID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxTraceID = TraceIDFromContext(r.Context())
		ctxSpanID = SpanIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Wrap in a mux router so normalizeRequestPath yields the route template.
	router := mux.NewRouter()
	router.Handle("/api/v1/items/{id}", Tracing(tracer, nil)(inner)).
		Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items/42", nil)
	req.Header.Set("User-Agent", "test-agent")
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]

	if span.SpanKind() != trace.SpanKindServer {
		t.Errorf("span kind = %v, want server", span.SpanKind())
	}
	if span.Name() != "GET /api/v1/items/{id}" {
		t.Errorf("span name = %q, want %q", span.Name(), "GET /api/v1/items/{id}")
	}

	attrs := map[string]string{}
	statusCode := int64(-1)
	for _, a := range span.Attributes() {
		switch a.Key {
		case "http.request.method", "http.route", "url.path",
			"server.address", "user_agent.original", "request.id":
			attrs[string(a.Key)] = a.Value.AsString()
		case "http.response.status_code":
			statusCode = a.Value.AsInt64()
		}
	}

	if attrs["http.request.method"] != http.MethodGet {
		t.Errorf("http.request.method = %q, want GET", attrs["http.request.method"])
	}
	if attrs["http.route"] != "/api/v1/items/{id}" {
		t.Errorf("http.route = %q, want /api/v1/items/{id}", attrs["http.route"])
	}
	if attrs["url.path"] != "/api/v1/items/42" {
		t.Errorf("url.path = %q, want /api/v1/items/42", attrs["url.path"])
	}
	if statusCode != http.StatusOK {
		t.Errorf("http.response.status_code = %d, want %d", statusCode, http.StatusOK)
	}

	// Context must carry the active trace/span identifiers.
	if ctxTraceID == "" {
		t.Error("trace_id should be set in context")
	}
	if ctxSpanID == "" {
		t.Error("span_id should be set in context")
	}
	if ctxTraceID != span.SpanContext().TraceID().String() {
		t.Errorf("ctx trace_id = %q, want %q",
			ctxTraceID, span.SpanContext().TraceID().String())
	}
}

func TestTracing_RecordsErrorStatusOn5xx(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantError  bool
	}{
		{"200 ok no error", http.StatusOK, false},
		{"404 client error no span error", http.StatusNotFound, false},
		{"500 marks span error", http.StatusInternalServerError, true},
		{"503 marks span error", http.StatusServiceUnavailable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer, sr := newRecordingTracer()

			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			})
			handler := Tracing(tracer, nil)(inner)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			spans := sr.Ended()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}

			isError := spans[0].Status().Code.String() == "Error"
			if isError != tt.wantError {
				t.Errorf("span error = %v, want %v (status %d)",
					isError, tt.wantError, tt.statusCode)
			}
		})
	}
}

func TestTracing_PropagatesInboundContext(t *testing.T) {
	// Arrange — build an inbound traceparent and assert the server span is a
	// child of it (same trace ID).
	tracer, sr := newRecordingTracer()
	propagator := propagation.TraceContext{}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := Tracing(tracer, propagator)(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// A well-formed W3C traceparent: version-traceid-spanid-flags.
	const inboundTraceID = "0af7651916cd43dd8448eb211c80319c"
	req.Header.Set("traceparent",
		"00-"+inboundTraceID+"-b7ad6b7169203331-01")
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].SpanContext().TraceID().String(); got != inboundTraceID {
		t.Errorf("server span trace id = %q, want inbound %q", got, inboundTraceID)
	}
	if !spans[0].Parent().IsValid() {
		t.Error("server span should have a valid parent from inbound context")
	}
}

func TestTracing_NoOpProviderSafe(t *testing.T) {
	// Arrange — a no-op tracer must not record spans nor set trace/span IDs.
	tracer := noop.NewTracerProvider().Tracer("noop")

	var ctxTraceID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxTraceID = TraceIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := Tracing(tracer, nil)(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert — passes through with 200 and no recorded trace id.
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ctxTraceID != "" {
		t.Errorf("trace_id should be empty under no-op tracer, got %q", ctxTraceID)
	}
}

func TestTraceIDFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "with trace id",
			ctx:  context.WithValue(context.Background(), TraceIDKey, "abc123"),
			want: "abc123",
		},
		{
			name: "without trace id",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "wrong type stored",
			ctx:  context.WithValue(context.Background(), TraceIDKey, 12345),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TraceIDFromContext(tt.ctx); got != tt.want {
				t.Errorf("TraceIDFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpanIDFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "with span id",
			ctx:  context.WithValue(context.Background(), SpanIDKey, "span-1"),
			want: "span-1",
		},
		{
			name: "without span id",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "wrong type stored",
			ctx:  context.WithValue(context.Background(), SpanIDKey, struct{}{}),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpanIDFromContext(tt.ctx); got != tt.want {
				t.Errorf("SpanIDFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}
