package observability

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

const (
	// serviceName is the OpenTelemetry service.name resource attribute.
	serviceName = "restapi-example"

	// exporterInitTimeout bounds the OTLP exporter connection establishment so
	// startup never blocks when the collector is unreachable. The gRPC/HTTP
	// exporters connect lazily, so this is a safety bound on resource setup.
	exporterInitTimeout = 5 * time.Second

	// tracerName is the instrumentation scope name used by the middleware.
	tracerName = "github.com/vyrodovalexey/restapi-example/internal/observability"
)

// TracingConfig holds the inputs required to initialize tracing. It is a small
// value type so callers do not need to depend on the full server config.
type TracingConfig struct {
	// OTLPEndpoint is the OTLP collector endpoint. When empty, a no-op tracer
	// provider is installed (no network calls, no errors, no-op shutdown).
	OTLPEndpoint string

	// ServiceVersion is the build version exposed as service.version. May be
	// empty.
	ServiceVersion string
}

// Provider owns the configured OpenTelemetry TracerProvider and its lifecycle.
// It is safe to use even when tracing is disabled: in that case Tracer returns
// a no-op tracer and Shutdown is a no-op.
type Provider struct {
	logger     *zap.Logger
	tp         trace.TracerProvider
	shutdownFn func(context.Context) error
	enabled    bool
}

// NewProvider creates an uninitialised Provider bound to the given logger.
// Call Init to configure it.
func NewProvider(logger *zap.Logger) *Provider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Provider{
		logger:     logger,
		tp:         noop.NewTracerProvider(),
		shutdownFn: func(context.Context) error { return nil },
	}
}

// Enabled reports whether OTLP tracing export is active. When false, the
// provider uses a no-op tracer.
func (p *Provider) Enabled() bool {
	return p.enabled
}

// Tracer returns a tracer from the configured provider. When tracing is
// disabled this returns a no-op tracer that incurs negligible overhead.
func (p *Provider) Tracer() trace.Tracer {
	return p.tp.Tracer(tracerName)
}

// Init configures the global OpenTelemetry TracerProvider and W3C propagators.
//
// When cfg.OTLPEndpoint is empty, it installs a no-op TracerProvider: no
// network connection is attempted, no error is returned, and Shutdown is a
// no-op. When set, it builds an OTel resource and a TracerProvider backed by a
// batch span processor and an OTLP exporter (gRPC by default, HTTP when the
// endpoint scheme is http/https). Exporter setup is bounded by a timeout so
// startup never blocks or fails if the collector is unreachable; the batch
// processor buffers spans and retries transparently.
func (p *Provider) Init(ctx context.Context, cfg TracingConfig) error {
	// Always set W3C propagators so trace context flows even when export is
	// disabled (callers may still want context propagation in logs).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if strings.TrimSpace(cfg.OTLPEndpoint) == "" {
		p.logger.Info("otlp tracing disabled (APP_OTLP_ENDPOINT unset); using no-op tracer")
		p.tp = noop.NewTracerProvider()
		p.enabled = false
		otel.SetTracerProvider(p.tp)
		return nil
	}

	res, err := buildResource(ctx, cfg.ServiceVersion)
	if err != nil {
		return fmt.Errorf("building otel resource: %w", err)
	}

	exporter, err := buildTraceExporter(ctx, cfg.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("building otlp trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	)

	p.tp = tp
	p.shutdownFn = tp.Shutdown
	p.enabled = true
	otel.SetTracerProvider(tp)

	p.logger.Info("otlp tracing enabled",
		zap.String("endpoint", cfg.OTLPEndpoint),
		zap.String("service_name", serviceName),
		zap.String("service_version", cfg.ServiceVersion),
	)

	return nil
}

// Shutdown flushes any buffered spans and releases exporter resources. It is
// bounded by the provided context deadline and is a no-op when tracing is
// disabled.
func (p *Provider) Shutdown(ctx context.Context) error {
	if !p.enabled {
		return nil
	}
	if err := p.shutdownFn(ctx); err != nil {
		return fmt.Errorf("shutting down tracer provider: %w", err)
	}
	p.logger.Info("otlp tracing shutdown complete")
	return nil
}

// buildResource constructs the OTel resource describing this service.
func buildResource(ctx context.Context, version string) (*resource.Resource, error) {
	attrs := []resource.Option{
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	}
	if version != "" {
		attrs = append(attrs, resource.WithAttributes(semconv.ServiceVersion(version)))
	}

	res, err := resource.New(ctx, attrs...)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}
	return res, nil
}

// buildTraceExporter creates an OTLP trace exporter. It selects the HTTP
// exporter when the endpoint carries an http/https scheme, otherwise it uses
// the gRPC exporter (the conventional OTLP transport). Insecure (non-TLS)
// transport is used for plain http:// and bare host:port endpoints, which are
// the common configurations for local/sidecar collectors. Setup is bounded by
// exporterInitTimeout so it never blocks startup.
func buildTraceExporter(ctx context.Context, endpoint string) (*otlptrace.Exporter, error) {
	initCtx, cancel := context.WithTimeout(ctx, exporterInitTimeout)
	defer cancel()

	scheme, host, insecure := parseEndpoint(endpoint)

	if scheme == "http" || scheme == "https" {
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(host)}
		if insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exp, err := otlptracehttp.New(initCtx, opts...)
		if err != nil {
			return nil, fmt.Errorf("creating otlp http exporter: %w", err)
		}
		return exp, nil
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(host)}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exp, err := otlptracegrpc.New(initCtx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating otlp grpc exporter: %w", err)
	}
	return exp, nil
}

// parseEndpoint normalises an OTLP endpoint into scheme, host:port, and whether
// the transport should be insecure. Bare "host:port" endpoints default to gRPC
// with insecure transport. https endpoints are treated as secure.
func parseEndpoint(endpoint string) (scheme, host string, insecure bool) {
	endpoint = strings.TrimSpace(endpoint)

	if !strings.Contains(endpoint, "://") {
		// Bare host:port — gRPC, insecure by default for local collectors.
		return "", endpoint, true
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", endpoint, true
	}

	host = u.Host
	if host == "" {
		host = u.Path
	}
	scheme = u.Scheme
	insecure = scheme != "https"
	return scheme, host, insecure
}
