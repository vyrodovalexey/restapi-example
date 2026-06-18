//go:build functional

package functional

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/observability"
)

// scrapeMetrics fetches the /metrics exposition and returns it as a string.
func scrapeMetrics(t *testing.T, client *HTTPClient) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	resp, err := client.Get(ctx, "/metrics", nil)
	if err != nil {
		t.Fatalf("Failed to scrape /metrics: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusOK)
	return string(resp.Body)
}

// counterValue extracts the value of a Prometheus counter/gauge sample whose
// line starts with metricLine (name + optional labels). Returns 0 if absent.
func counterValue(t *testing.T, metrics, metricLine string) float64 {
	t.Helper()
	re := regexp.MustCompile(
		`(?m)^` + regexp.QuoteMeta(metricLine) + `\s+([0-9.eE+-]+)$`,
	)
	m := re.FindStringSubmatch(metrics)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("Failed to parse metric value %q: %v", m[1], err)
	}
	return v
}

// TestFunctional_MET_EXIST asserts the full extended metric set is exposed on
// the /metrics endpoint after the server has handled some requests.
// MET-EXIST.
func TestFunctional_MET_EXIST(t *testing.T) {
	LogTestStart(t, "FT-MET-EXIST", "Extended metric set present in /metrics")
	defer LogTestEnd(t, "FT-MET-EXIST")

	ts := NewMetricsTestServer(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	// Populate build_info as production does via ldflags wiring.
	observability.SetBuildInfo("functional-test", "test-commit", "test-time")

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	headers := APIKeyHeaders(testAPIKey)

	// Exercise REST (success), a store create, and a failed auth so that the
	// labelled counters appear in the exposition.
	_, _ = client.Get(ctx, "/api/v1/items", headers)
	_, _ = client.Post(ctx, "/api/v1/items", CreateItemRequest{
		Name: "Metric Item", Price: 1.0,
	}, headers)
	_, _ = client.Get(ctx, "/api/v1/items", APIKeyHeaders("bogus-key"))

	metrics := scrapeMetrics(t, client)

	required := []string{
		// Pre-existing HTTP metrics.
		"http_requests_total",
		"http_request_duration_seconds",
		"http_requests_in_flight",
		// New domain metrics.
		"auth_attempts_total",
		"store_operations_total",
		"store_operation_duration_seconds",
		"panics_recovered_total",
		"http_response_size_bytes",
		"build_info",
		// Runtime + process collectors.
		"go_goroutines",
		"process_cpu_seconds_total",
	}
	for _, name := range required {
		if !strings.Contains(metrics, name) {
			t.Errorf("MET-EXIST: /metrics missing %q", name)
		}
	}

	// build_info should carry the labels we set.
	if !strings.Contains(metrics, `build_info{`) ||
		!strings.Contains(metrics, `version="functional-test"`) {
		t.Errorf("MET-EXIST: build_info missing expected version label")
	}
}

// TestFunctional_MET_AUTH asserts auth_attempts_total increments for both
// success and failure paths. MET-AUTH-OK / MET-AUTH-NG.
func TestFunctional_MET_AUTH(t *testing.T) {
	LogTestStart(t, "FT-MET-AUTH", "auth_attempts_total increments")
	defer LogTestEnd(t, "FT-MET-AUTH")

	ts := NewMetricsTestServer(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	const successLine = `auth_attempts_total{method="apikey",result="success"}`
	const failureLine = `auth_attempts_total{method="apikey",result="failure"}`

	before := scrapeMetrics(t, client)
	successBefore := counterValue(t, before, successLine)
	failureBefore := counterValue(t, before, failureLine)

	// One successful auth.
	resp, err := client.Get(ctx, "/api/v1/items", APIKeyHeaders(testAPIKey))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusOK)

	// One failed auth.
	resp, err = client.Get(ctx, "/api/v1/items", APIKeyHeaders("wrong-key"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusUnauthorized)

	after := scrapeMetrics(t, client)
	successAfter := counterValue(t, after, successLine)
	failureAfter := counterValue(t, after, failureLine)

	if successAfter < successBefore+1 {
		t.Errorf(
			"MET-AUTH-OK: success counter did not increment: before=%v after=%v",
			successBefore, successAfter,
		)
	}
	if failureAfter < failureBefore+1 {
		t.Errorf(
			"MET-AUTH-NG: failure counter did not increment: before=%v after=%v",
			failureBefore, failureAfter,
		)
	}
}

// TestFunctional_MET_DISABLE asserts that when metrics are disabled the
// /metrics route is absent from the main router. MET-DISABLE.
func TestFunctional_MET_DISABLE(t *testing.T) {
	LogTestStart(t, "FT-MET-DISABLE", "metrics route absent when disabled")
	defer LogTestEnd(t, "FT-MET-DISABLE")

	// NewTestServer uses MetricsEnabled=false by default (DefaultMetricsEnabled).
	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	resp, err := client.Get(ctx, "/metrics", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound &&
		resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf(
			"MET-DISABLE: expected 404/405 for /metrics when disabled, got %d",
			resp.StatusCode,
		)
	}
}
