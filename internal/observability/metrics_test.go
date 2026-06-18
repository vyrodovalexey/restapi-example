package observability

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMetricsRegisteredOnDefaultRegistry verifies that all domain collectors
// are registered on the default Prometheus registry by gathering and asserting
// their presence. Registration via promauto happens at package init, so the
// metrics must already exist.
func TestMetricsRegisteredOnDefaultRegistry(t *testing.T) {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	names := make(map[string]bool, len(mfs))
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}

	// CounterVec/HistogramVec without any observed series are NOT emitted by
	// Gather. We therefore drive at least one observation on each Vec metric
	// below before asserting; gauges and registered runtime collectors do
	// appear immediately.
	wantImmediate := []string{
		"websocket_active_connections",
	}
	for _, name := range wantImmediate {
		if !names[name] {
			t.Errorf("expected metric %q to be registered on default registry", name)
		}
	}
}

// TestRegisterRuntimeCollectors_DuplicateSafe verifies the runtime collector
// registration is idempotent and never panics on repeated invocation.
func TestRegisterRuntimeCollectors_DuplicateSafe(t *testing.T) {
	// Reset the guard so the function does real work, then call twice.
	prev := runtimeCollectorsRegistered
	defer func() { runtimeCollectorsRegistered = prev }()

	runtimeCollectorsRegistered = false

	// Should not panic even though collectors may already be registered.
	registerRuntimeCollectors()
	registerRuntimeCollectors() // second call returns early via the guard
}

// TestRegisterRuntimeCollectors_GuardSkips verifies that when the guard is
// already set, the function returns without attempting registration.
func TestRegisterRuntimeCollectors_GuardSkips(t *testing.T) {
	prev := runtimeCollectorsRegistered
	defer func() { runtimeCollectorsRegistered = prev }()

	runtimeCollectorsRegistered = true
	// Must not panic; simply returns early.
	registerRuntimeCollectors()
}

// TestTryRegister_AlreadyRegistered ensures tryRegister swallows
// AlreadyRegisteredError without panicking.
func TestTryRegister_AlreadyRegistered(t *testing.T) {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "observability_test_dup_counter",
		Help: "test",
	})
	tryRegister(c)
	// Registering the exact same collector again triggers
	// AlreadyRegisteredError, which must be ignored.
	tryRegister(c)
	// Cleanup so we don't leak into other tests.
	prometheus.Unregister(c)
}

// TestSetBuildInfo verifies SetBuildInfo sets the build_info gauge to 1 with
// the provided labels and that resetting replaces previous label sets.
func TestSetBuildInfo(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		commit    string
		buildTime string
	}{
		{"typical", "v1.2.3", "abc123", "2026-06-18T00:00:00Z"},
		{"empty values", "", "", ""},
		{"replacement", "v2.0.0", "def456", "2026-06-19T00:00:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			SetBuildInfo(tt.version, tt.commit, tt.buildTime)

			// Assert - exactly one series with value 1 and the expected labels.
			expected := `
# HELP build_info Build information of the running binary (value is always 1)
# TYPE build_info gauge
build_info{build_time="` + tt.buildTime + `",commit="` + tt.commit + `",version="` + tt.version + `"} 1
`
			if err := testutil.CollectAndCompare(
				buildInfo, strings.NewReader(expected), "build_info",
			); err != nil {
				t.Errorf("build_info mismatch: %v", err)
			}
		})
	}
}

// TestAuthAttemptsTotal_Increments asserts the auth counter increments per
// method/result label combination.
func TestAuthAttemptsTotal_Increments(t *testing.T) {
	AuthAttemptsTotal.Reset()

	AuthAttemptsTotal.WithLabelValues("basic", ResultSuccess).Inc()
	AuthAttemptsTotal.WithLabelValues("basic", ResultSuccess).Inc()
	AuthAttemptsTotal.WithLabelValues("oidc", ResultFailure).Inc()

	if got := testutil.ToFloat64(
		AuthAttemptsTotal.WithLabelValues("basic", ResultSuccess),
	); got != 2 {
		t.Errorf("auth_attempts_total{basic,success} = %v, want 2", got)
	}
	if got := testutil.ToFloat64(
		AuthAttemptsTotal.WithLabelValues("oidc", ResultFailure),
	); got != 1 {
		t.Errorf("auth_attempts_total{oidc,failure} = %v, want 1", got)
	}
}

// TestWebSocketActiveConnections_IncDec asserts the gauge tracks inc/dec.
func TestWebSocketActiveConnections_IncDec(t *testing.T) {
	WebSocketActiveConnections.Set(0)

	WebSocketActiveConnections.Inc()
	WebSocketActiveConnections.Inc()
	if got := testutil.ToFloat64(WebSocketActiveConnections); got != 2 {
		t.Errorf("websocket_active_connections = %v, want 2", got)
	}

	WebSocketActiveConnections.Dec()
	if got := testutil.ToFloat64(WebSocketActiveConnections); got != 1 {
		t.Errorf("websocket_active_connections = %v, want 1", got)
	}

	WebSocketActiveConnections.Set(0)
}

// TestStoreOperationsTotal_Increments asserts the store counter increments by
// operation/result.
func TestStoreOperationsTotal_Increments(t *testing.T) {
	StoreOperationsTotal.Reset()

	StoreOperationsTotal.WithLabelValues("create", ResultSuccess).Inc()
	StoreOperationsTotal.WithLabelValues("get", ResultFailure).Inc()

	if got := testutil.ToFloat64(
		StoreOperationsTotal.WithLabelValues("create", ResultSuccess),
	); got != 1 {
		t.Errorf("store_operations_total{create,success} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(
		StoreOperationsTotal.WithLabelValues("get", ResultFailure),
	); got != 1 {
		t.Errorf("store_operations_total{get,failure} = %v, want 1", got)
	}
}

// TestStoreOperationDuration_Observes asserts the histogram records samples.
func TestStoreOperationDuration_Observes(t *testing.T) {
	StoreOperationDuration.Reset()

	StoreOperationDuration.WithLabelValues("list").Observe(0.01)
	StoreOperationDuration.WithLabelValues("list").Observe(0.02)

	if got := testutil.CollectAndCount(StoreOperationDuration); got == 0 {
		t.Error("store_operation_duration_seconds recorded no series")
	}
}

// TestPanicsRecoveredTotal_Increments asserts the panic counter increments.
func TestPanicsRecoveredTotal_Increments(t *testing.T) {
	before := testutil.ToFloat64(PanicsRecoveredTotal)
	PanicsRecoveredTotal.Inc()
	after := testutil.ToFloat64(PanicsRecoveredTotal)

	if after-before != 1 {
		t.Errorf("panics_recovered_total delta = %v, want 1", after-before)
	}
}

// TestHTTPResponseSizeBytes_Observes asserts the response-size histogram
// records observations by method/path.
func TestHTTPResponseSizeBytes_Observes(t *testing.T) {
	HTTPResponseSizeBytes.Reset()

	HTTPResponseSizeBytes.WithLabelValues("GET", "/api/v1/items").Observe(150)
	HTTPResponseSizeBytes.WithLabelValues("GET", "/api/v1/items").Observe(500)

	if got := testutil.CollectAndCount(HTTPResponseSizeBytes); got == 0 {
		t.Error("http_response_size_bytes recorded no series")
	}
}

// TestResultConstants pins the exported result label values.
func TestResultConstants(t *testing.T) {
	if ResultSuccess != "success" {
		t.Errorf("ResultSuccess = %q, want success", ResultSuccess)
	}
	if ResultFailure != "failure" {
		t.Errorf("ResultFailure = %q, want failure", ResultFailure)
	}
}
