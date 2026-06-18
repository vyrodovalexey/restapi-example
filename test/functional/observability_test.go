//go:build functional

package functional

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/observability"
)

// TestFunctional_OT_NOOP_Safety verifies that the server operates normally
// when APP_OTLP_ENDPOINT is unset: the tracing middleware runs as a no-op,
// requests succeed, and no errors surface. OT-NOOP-1 / OT-NOOP-2 / OT-NOOP-3.
func TestFunctional_OT_NOOP_Safety(t *testing.T) {
	LogTestStart(t, "FT-OT-NOOP", "OTLP no-op safety with endpoint unset")
	defer LogTestEnd(t, "FT-OT-NOOP")

	// Ensure the OTLP endpoint is unset for the duration of this test so the
	// no-op tracer provider is in effect.
	if v, ok := os.LookupEnv("APP_OTLP_ENDPOINT"); ok {
		t.Setenv("APP_OTLP_ENDPOINT", "")
		t.Logf("temporarily cleared APP_OTLP_ENDPOINT (was %q)", v)
	}

	// OT-NOOP-1 / OT-NOOP-3: a provider initialised with an empty endpoint is
	// a no-op and Shutdown returns immediately without error.
	ctx := context.Background()
	provider := observability.NewProvider(nil)
	if err := provider.Init(ctx, observability.TracingConfig{
		OTLPEndpoint:   "",
		ServiceVersion: "functional-test",
	}); err != nil {
		t.Fatalf("OT-NOOP-1: expected no error with empty endpoint, got %v", err)
	}
	if provider.Enabled() {
		t.Error("OT-NOOP-1: provider should be disabled (no-op) when endpoint unset")
	}
	// Tracer must be usable (no-op) and not panic.
	if provider.Tracer() == nil {
		t.Error("OT-NOOP-1: Tracer() returned nil")
	}
	if err := provider.Shutdown(ctx); err != nil {
		t.Errorf("OT-NOOP-3: Shutdown should be a no-op, got %v", err)
	}

	// OT-NOOP-2: serve real requests with tracing as the no-op provider.
	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	reqCtx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	// REST.
	resp, err := client.Post(reqCtx, "/api/v1/items", CreateItemRequest{
		Name: "OTLP NoOp Item", Price: 5.0,
	}, nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusCreated)

	// List back.
	resp, err = client.Get(reqCtx, "/api/v1/items", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusOK)

	// Health.
	resp, err = client.Get(reqCtx, "/health", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusOK)

	t.Log("server operated normally with OTLP disabled (no-op tracing)")
}
