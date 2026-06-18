//go:build functional

package functional

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

// mtlsDoRequest performs an HTTP request with a provided mTLS client and
// returns the status code and body.
func mtlsDoRequest(
	t *testing.T,
	client *http.Client,
	method, url string,
	body io.Reader,
) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request %s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// TestFunctional_AUTH_MTLS_ValidCert verifies that a valid Vault-issued client
// certificate authenticates successfully and can drive REST CRUD over mTLS.
// AM-MTLS-1.
func TestFunctional_AUTH_MTLS_ValidCert(t *testing.T) {
	LogTestStart(t, "FT-AUTH-MTLS-1", "mTLS valid client cert")
	defer LogTestEnd(t, "FT-AUTH-MTLS-1")

	ts := NewFunctionalMTLSServer(t)

	dir := funcCertsDir()
	client, err := funcTLSClient(
		filepath.Join(dir, "ca-cert.pem"),
		filepath.Join(dir, "test-client-cert.pem"),
		filepath.Join(dir, "test-client-key.pem"),
	)
	if err != nil {
		t.Fatalf("Failed to build mTLS client: %v", err)
	}

	// List items succeeds with a valid client cert.
	status, body := mtlsDoRequest(
		t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
	)
	if status != http.StatusOK {
		t.Fatalf("AM-MTLS-1: expected 200, got %d. Body: %s", status, body)
	}

	// Create an item over mTLS.
	createBody, _ := json.Marshal(map[string]any{
		"name": "mTLS Functional Item", "price": 12.5,
	})
	status, body = mtlsDoRequest(
		t, client, http.MethodPost, ts.baseURL+"/api/v1/items",
		bytes.NewReader(createBody),
	)
	if status != http.StatusCreated {
		t.Fatalf("AM-MTLS-1 create: expected 201, got %d. Body: %s", status, body)
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	if resp.Data.ID == "" {
		t.Fatal("AM-MTLS-1: created item has empty ID")
	}

	// Read it back.
	status, _ = mtlsDoRequest(
		t, client, http.MethodGet,
		fmt.Sprintf("%s/api/v1/items/%s", ts.baseURL, resp.Data.ID), nil,
	)
	if status != http.StatusOK {
		t.Errorf("AM-MTLS-1 read: expected 200, got %d", status)
	}
}

// TestFunctional_AUTH_MTLS_NoCert verifies that a client presenting no
// certificate is rejected at the TLS handshake (client-auth=require).
// AM-MTLS-2.
func TestFunctional_AUTH_MTLS_NoCert(t *testing.T) {
	LogTestStart(t, "FT-AUTH-MTLS-2", "mTLS no client cert rejected")
	defer LogTestEnd(t, "FT-AUTH-MTLS-2")

	ts := NewFunctionalMTLSServer(t)

	dir := funcCertsDir()
	client, err := funcNoCertTLSClient(filepath.Join(dir, "ca-cert.pem"))
	if err != nil {
		t.Fatalf("Failed to build no-cert TLS client: %v", err)
	}

	_, err = client.Get(ts.baseURL + "/api/v1/items")
	if err == nil {
		t.Errorf("AM-MTLS-2: expected handshake failure without client cert")
	} else {
		t.Logf("AM-MTLS-2: handshake correctly rejected: %v", err)
	}
}

// TestFunctional_AUTH_MTLS_PublicEndpointsRequireCert verifies that even the
// probe endpoints require a client cert at the TLS layer when the server is in
// require mode (TLS auth happens before HTTP routing).
func TestFunctional_AUTH_MTLS_HealthWithCert(t *testing.T) {
	LogTestStart(t, "FT-AUTH-MTLS-3", "mTLS health reachable with cert")
	defer LogTestEnd(t, "FT-AUTH-MTLS-3")

	ts := NewFunctionalMTLSServer(t)

	dir := funcCertsDir()
	client, err := funcTLSClient(
		filepath.Join(dir, "ca-cert.pem"),
		filepath.Join(dir, "test-client-cert.pem"),
		filepath.Join(dir, "test-client-key.pem"),
	)
	if err != nil {
		t.Fatalf("Failed to build mTLS client: %v", err)
	}

	status, _ := mtlsDoRequest(
		t, client, http.MethodGet, ts.baseURL+"/health", nil,
	)
	if status != http.StatusOK {
		t.Errorf("expected 200 for /health over mTLS, got %d", status)
	}
}
