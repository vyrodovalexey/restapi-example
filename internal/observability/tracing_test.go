package observability

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

func TestNewProvider_NilLogger(t *testing.T) {
	// A nil logger must be tolerated (replaced with a no-op logger).
	p := NewProvider(nil)
	if p == nil {
		t.Fatal("NewProvider(nil) returned nil")
	}
	if p.logger == nil {
		t.Error("logger should be set to a no-op logger when nil is passed")
	}
	if p.Enabled() {
		t.Error("freshly constructed provider should not be enabled")
	}
	// Tracer must be usable immediately (no-op).
	if p.Tracer() == nil {
		t.Error("Tracer() returned nil on uninitialised provider")
	}
}

func TestNewProvider_WithLogger(t *testing.T) {
	logger := zap.NewExample()
	p := NewProvider(logger)
	if p == nil {
		t.Fatal("NewProvider returned nil")
	}
	if p.logger != logger {
		t.Error("provided logger should be retained")
	}
}

func TestParseEndpoint_MalformedURL(t *testing.T) {
	// A scheme-bearing but malformed URL must fall back to treating the raw
	// string as the host with insecure transport.
	scheme, host, insecure := parseEndpoint("http://[::1]:namedport")
	if host == "" {
		t.Error("expected non-empty host fallback for malformed URL")
	}
	if !insecure {
		t.Error("malformed URL should default to insecure transport")
	}
	_ = scheme
}

func TestParseEndpoint_SchemeOnlyHostInPath(t *testing.T) {
	// When url.Host is empty (e.g. the value lands in Path), the host should be
	// taken from the path.
	scheme, host, _ := parseEndpoint("http://collector:4318")
	if scheme != "http" {
		t.Errorf("scheme = %q, want http", scheme)
	}
	if host != "collector:4318" {
		t.Errorf("host = %q, want collector:4318", host)
	}
}

func TestProvider_Init_NoOpWhenEndpointUnset(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"tab and spaces", "\t  \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider(zap.NewNop())
			ctx := context.Background()

			err := p.Init(ctx, TracingConfig{OTLPEndpoint: tt.endpoint})
			if err != nil {
				t.Fatalf("Init() error = %v, want nil", err)
			}
			if p.Enabled() {
				t.Error("provider should be disabled when endpoint is unset")
			}

			// Shutdown must be an immediate no-op returning nil.
			done := make(chan error, 1)
			go func() { done <- p.Shutdown(ctx) }()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Shutdown() error = %v, want nil", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Shutdown() blocked for a disabled provider")
			}

			// Global propagator must be set to W3C TraceContext + Baggage.
			if otel.GetTextMapPropagator() == nil {
				t.Error("expected a global text map propagator to be set")
			}
		})
	}
}

func TestProvider_Init_PropagatorsSet(t *testing.T) {
	p := NewProvider(zap.NewNop())
	if err := p.Init(context.Background(), TracingConfig{}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	prop := otel.GetTextMapPropagator()
	fields := prop.Fields()

	// W3C TraceContext contributes "traceparent"; Baggage contributes
	// "baggage".
	hasTraceparent := false
	hasBaggage := false
	for _, f := range fields {
		switch f {
		case "traceparent":
			hasTraceparent = true
		case "baggage":
			hasBaggage = true
		}
	}
	if !hasTraceparent {
		t.Error("expected traceparent propagation field to be configured")
	}
	if !hasBaggage {
		t.Error("expected baggage propagation field to be configured")
	}
}

func TestProvider_Init_EnabledDoesNotBlockWhenCollectorUnreachable(t *testing.T) {
	// Use a bogus, definitely-unreachable endpoint. The gRPC/HTTP OTLP
	// exporters connect lazily, so Init must return promptly without blocking
	// even though no collector is listening.
	p := NewProvider(zap.NewNop())
	ctx := context.Background()

	done := make(chan error, 1)
	go func() {
		done <- p.Init(ctx, TracingConfig{
			OTLPEndpoint:   "127.0.0.1:9",
			ServiceVersion: "v9.9.9",
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Init() with unreachable collector returned error = %v", err)
		}
	case <-time.After(exporterInitTimeout + 5*time.Second):
		t.Fatal("Init() blocked when collector was unreachable")
	}

	if !p.Enabled() {
		t.Error("provider should be enabled when an endpoint is configured")
	}

	// Shutdown should flush/return within a deadline.
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), 5*time.Second,
	)
	defer cancel()

	sdone := make(chan error, 1)
	go func() { sdone <- p.Shutdown(shutdownCtx) }()
	select {
	case <-sdone:
		// returned within deadline (error or nil both acceptable here)
	case <-time.After(8 * time.Second):
		t.Fatal("Shutdown() did not return within the deadline")
	}
}

func TestProvider_Shutdown_DisabledIsNoOp(t *testing.T) {
	p := NewProvider(zap.NewNop())
	// Not initialised / disabled.
	if err := p.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() on disabled provider = %v, want nil", err)
	}
}

func TestProvider_Shutdown_PropagatesError(t *testing.T) {
	// Force an enabled provider whose shutdown function returns an error to
	// exercise the error-wrapping branch.
	wantErr := context.DeadlineExceeded
	p := &Provider{
		logger:     zap.NewNop(),
		tp:         noop.NewTracerProvider(),
		enabled:    true,
		shutdownFn: func(context.Context) error { return wantErr },
	}

	err := p.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() = nil, want wrapped error")
	}
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		wantScheme   string
		wantHost     string
		wantInsecure bool
	}{
		{
			name:         "bare host port defaults to grpc insecure",
			endpoint:     "localhost:4317",
			wantScheme:   "",
			wantHost:     "localhost:4317",
			wantInsecure: true,
		},
		{
			name:         "http endpoint insecure",
			endpoint:     "http://collector:4318",
			wantScheme:   "http",
			wantHost:     "collector:4318",
			wantInsecure: true,
		},
		{
			name:         "https endpoint secure",
			endpoint:     "https://collector:4318",
			wantScheme:   "https",
			wantHost:     "collector:4318",
			wantInsecure: false,
		},
		{
			name:         "grpc scheme treated as non-http insecure",
			endpoint:     "grpc://otel:4317",
			wantScheme:   "grpc",
			wantHost:     "otel:4317",
			wantInsecure: true,
		},
		{
			name:         "trims surrounding whitespace",
			endpoint:     "  localhost:4317  ",
			wantScheme:   "",
			wantHost:     "localhost:4317",
			wantInsecure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, host, insecure := parseEndpoint(tt.endpoint)
			if scheme != tt.wantScheme {
				t.Errorf("scheme = %q, want %q", scheme, tt.wantScheme)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
			if insecure != tt.wantInsecure {
				t.Errorf("insecure = %v, want %v", insecure, tt.wantInsecure)
			}
		})
	}
}

func TestBuildResource(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"with version", "v1.0.0"},
		{"without version", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := buildResource(context.Background(), tt.version)
			if err != nil {
				t.Fatalf("buildResource() error = %v", err)
			}
			if res == nil {
				t.Fatal("buildResource() returned nil resource")
			}

			foundServiceName := false
			for _, attr := range res.Attributes() {
				if string(attr.Key) == "service.name" {
					foundServiceName = true
					if attr.Value.AsString() != serviceName {
						t.Errorf("service.name = %q, want %q",
							attr.Value.AsString(), serviceName)
					}
				}
			}
			if !foundServiceName {
				t.Error("resource missing service.name attribute")
			}
		})
	}
}

func TestBuildTraceExporter_HTTPAndGRPC(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{"http exporter", "http://localhost:4318"},
		{"https exporter", "https://localhost:4318"},
		{"grpc bare host", "localhost:4317"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			exp, err := buildTraceExporter(ctx, tt.endpoint)
			if err != nil {
				t.Fatalf("buildTraceExporter() error = %v", err)
			}
			if exp == nil {
				t.Fatal("buildTraceExporter() returned nil exporter")
			}
			// Shut the exporter down to release resources.
			shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			_ = exp.Shutdown(shutdownCtx)
		})
	}
}

// TestProvider_Tracer_UsableUnderNoOp verifies a span can be started/ended on a
// disabled provider without panic.
func TestProvider_Tracer_UsableUnderNoOp(t *testing.T) {
	p := NewProvider(zap.NewNop())
	if err := p.Init(context.Background(), TracingConfig{}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	_, span := p.Tracer().Start(context.Background(), "noop-span")
	span.End()
}

// ensure propagation import is exercised (compile-time guard for header carrier
// usage in the wider package).
var _ = propagation.HeaderCarrier{}
