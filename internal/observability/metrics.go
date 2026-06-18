// Package observability provides Prometheus metrics and OpenTelemetry tracing
// for the REST API server. It centralizes telemetry collectors and exposes a
// Provider that wires OTLP tracing behind configuration with a safe no-op
// default when tracing is disabled.
package observability

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metric label and result constants. Centralized to avoid duplicated literals
// and to keep label cardinality bounded and consistent across the codebase.
const (
	// ResultSuccess is the result label value for a successful operation.
	ResultSuccess = "success"
	// ResultFailure is the result label value for a failed operation.
	ResultFailure = "failure"

	labelMethod    = "method"
	labelResult    = "result"
	labelOperation = "operation"
	labelPath      = "path"
	labelVersion   = "version"
	labelCommit    = "commit"
	labelBuildTime = "build_time"
)

// Domain and runtime Prometheus metrics.
//
// The following collectors are registered on the default Prometheus registry
// via promauto. The pre-existing HTTP metrics (http_requests_total,
// http_request_duration_seconds, http_requests_in_flight) remain defined in
// the middleware package and are intentionally left unchanged.
var (
	// AuthAttemptsTotal counts authentication attempts.
	// Labels:
	//   method - the authentication method (mtls|basic|apikey|oidc|multi).
	//   result - success|failure.
	AuthAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_attempts_total",
			Help: "Total number of authentication attempts by method and result",
		},
		[]string{labelMethod, labelResult},
	)

	// WebSocketActiveConnections tracks the number of live WebSocket
	// connections. Incremented on register, decremented on unregister.
	WebSocketActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "websocket_active_connections",
			Help: "Number of currently active WebSocket connections",
		},
	)

	// StoreOperationsTotal counts store operations.
	// Labels:
	//   operation - list|get|create|update|delete.
	//   result    - success|failure.
	StoreOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "store_operations_total",
			Help: "Total number of store operations by operation and result",
		},
		[]string{labelOperation, labelResult},
	)

	// StoreOperationDuration observes store operation latency in seconds.
	// Label:
	//   operation - list|get|create|update|delete.
	StoreOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "store_operation_duration_seconds",
			Help:    "Store operation duration in seconds by operation",
			Buckets: prometheus.DefBuckets,
		},
		[]string{labelOperation},
	)

	// PanicsRecoveredTotal counts panics recovered by the Recovery middleware.
	PanicsRecoveredTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "panics_recovered_total",
			Help: "Total number of panics recovered by the recovery middleware",
		},
	)

	// HTTPResponseSizeBytes observes HTTP response body sizes in bytes.
	// Labels:
	//   method - the HTTP request method.
	//   path   - the normalized route template (never the raw path) to keep
	//            label cardinality bounded.
	HTTPResponseSizeBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "http_response_size_bytes",
			Help: "HTTP response body size in bytes by method and route template",
			// Buckets spanning 100B .. ~25MB.
			Buckets: prometheus.ExponentialBuckets(100, 4, 8),
		},
		[]string{labelMethod, labelPath},
	)

	// buildInfo is a constant gauge (value 1) carrying build metadata labels.
	// Labels: version, commit, build_time.
	buildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "build_info",
			Help: "Build information of the running binary (value is always 1)",
		},
		[]string{labelVersion, labelCommit, labelBuildTime},
	)
)

// runtimeCollectorsOnce guards one-time registration of runtime collectors.
var runtimeCollectorsRegistered bool

// init ensures the Go runtime and process collectors are registered on the
// default registry. promauto's default registry already includes a
// GoCollector and ProcessCollector, but we register the process collector
// explicitly and guard against duplicate registration to be robust across
// client_golang versions.
func init() {
	registerRuntimeCollectors()
}

// registerRuntimeCollectors registers the Go runtime and process collectors,
// silently ignoring AlreadyRegisteredError so that it is safe regardless of
// what the default registry already auto-registers.
func registerRuntimeCollectors() {
	if runtimeCollectorsRegistered {
		return
	}
	runtimeCollectorsRegistered = true

	tryRegister(collectors.NewGoCollector())
	tryRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

// tryRegister registers a collector, tolerating duplicate registration.
// Any error (including AlreadyRegisteredError) is non-fatal for telemetry: the
// default registry already having an equivalent collector is acceptable, so
// registration errors are deliberately swallowed and never affect availability.
func tryRegister(c prometheus.Collector) {
	err := prometheus.Register(c)
	if err == nil {
		return
	}

	var are prometheus.AlreadyRegisteredError
	if errors.As(err, &are) {
		// Collector (or an equivalent) is already registered — nothing to do.
		return
	}
	// Any other error is intentionally ignored to keep telemetry best-effort.
}

// SetBuildInfo sets the build_info gauge to 1 with the provided build metadata
// labels. It should be called once at startup with the values injected via
// -ldflags (main.Version/Commit/BuildTime).
func SetBuildInfo(version, commit, buildTime string) {
	buildInfo.Reset()
	buildInfo.WithLabelValues(version, commit, buildTime).Set(1)
}
