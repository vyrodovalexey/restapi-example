//go:build integration

package integration_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

// serverURL returns the base URL of the server under test.
func serverURL() string {
	return getEnvOrDefault(EnvServerURL, DefaultServerURL)
}

// TestIntegration_HealthEndpointAccessible verifies that GET /health
// returns HTTP 200 with a healthy status.
func TestIntegration_HealthEndpointAccessible(t *testing.T) {
	t.Parallel()

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()
	status, body := doRequest(
		t, client, http.MethodGet, base+"/health", nil, nil,
	)

	if status != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", status, body)
	}

	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success=true, got false")
	}

	var health healthResponse
	if err := json.Unmarshal(resp.Data, &health); err != nil {
		t.Fatalf("Failed to parse health data: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %q", health.Status)
	}

	t.Logf("Health check passed: status=%s version=%s",
		health.Status, health.Version)
}

// TestIntegration_ReadyEndpointAccessible verifies that GET /ready
// returns HTTP 200 with a ready status.
func TestIntegration_ReadyEndpointAccessible(t *testing.T) {
	t.Parallel()

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()
	status, body := doRequest(
		t, client, http.MethodGet, base+"/ready", nil, nil,
	)

	if status != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", status, body)
	}

	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success=true, got false")
	}

	var ready readyResponse
	if err := json.Unmarshal(resp.Data, &ready); err != nil {
		t.Fatalf("Failed to parse ready data: %v", err)
	}

	if ready.Status != "ready" {
		t.Errorf("Expected status 'ready', got %q", ready.Status)
	}

	t.Logf("Ready check passed: status=%s", ready.Status)
}

// TestIntegration_MetricsEndpointAccessible verifies that GET /metrics
// returns HTTP 200 with Prometheus-formatted metrics.
func TestIntegration_MetricsEndpointAccessible(t *testing.T) {
	t.Parallel()

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()
	status, body := doRequest(
		t, client, http.MethodGet, base+"/metrics", nil, nil,
	)

	// Metrics may be disabled; skip if 404 or 405.
	if status == http.StatusNotFound ||
		status == http.StatusMethodNotAllowed {
		t.Skip("Metrics endpoint not enabled on server")
	}

	if status != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", status, body)
	}

	// Prometheus metrics should contain at least one HELP line.
	if !strings.Contains(string(body), "# HELP") {
		t.Error("Expected Prometheus metrics format with # HELP")
	}

	t.Log("Metrics endpoint accessible and returning data")
}

// TestIntegration_CRUDOperations exercises the full Create, Read,
// Update, Delete lifecycle against the running server.
func TestIntegration_CRUDOperations(t *testing.T) {
	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()
	headers := buildAuthHeaders(t)
	headers["Content-Type"] = "application/json"

	// --- Create ---
	t.Log("Step 1: Create item")
	createBody, _ := json.Marshal(map[string]any{
		"name":        "Integration Test Item",
		"description": "Created by integration test",
		"price":       42.00,
	})

	status, body := doRequest(
		t, client, http.MethodPost,
		base+"/api/v1/items",
		bytes.NewReader(createBody), headers,
	)

	if status != http.StatusCreated {
		t.Fatalf("Create: expected 201, got %d. Body: %s",
			status, body)
	}

	var createResp apiResponse
	if err := json.Unmarshal(body, &createResp); err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	var created itemResponse
	if err := json.Unmarshal(createResp.Data, &created); err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	if created.ID == "" {
		t.Fatal("Created item has empty ID")
	}
	t.Logf("Created item ID=%s", created.ID)

	// --- Read ---
	t.Log("Step 2: Read item")
	itemURL := fmt.Sprintf(
		"%s/api/v1/items/%s", base, created.ID,
	)
	status, body = doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	if status != http.StatusOK {
		t.Fatalf("Read: expected 200, got %d. Body: %s",
			status, body)
	}

	var readResp apiResponse
	if err := json.Unmarshal(body, &readResp); err != nil {
		t.Fatalf("Failed to parse read response: %v", err)
	}

	var readItem itemResponse
	if err := json.Unmarshal(readResp.Data, &readItem); err != nil {
		t.Fatalf("Failed to parse read item: %v", err)
	}

	if readItem.Name != "Integration Test Item" {
		t.Errorf(
			"Read: expected name 'Integration Test Item', got %q",
			readItem.Name,
		)
	}

	// --- Update ---
	t.Log("Step 3: Update item")
	updateBody, _ := json.Marshal(map[string]any{
		"name":        "Updated Integration Item",
		"description": "Updated by integration test",
		"price":       84.00,
	})

	status, body = doRequest(
		t, client, http.MethodPut, itemURL,
		bytes.NewReader(updateBody), headers,
	)

	if status != http.StatusOK {
		t.Fatalf("Update: expected 200, got %d. Body: %s",
			status, body)
	}

	// Verify update
	status, body = doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	var verifyResp apiResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		t.Fatalf("Failed to parse verify response: %v", err)
	}

	var verifyItem itemResponse
	if err := json.Unmarshal(verifyResp.Data, &verifyItem); err != nil {
		t.Fatalf("Failed to parse verify item: %v", err)
	}

	if verifyItem.Name != "Updated Integration Item" {
		t.Errorf(
			"Update verify: expected 'Updated Integration Item', got %q",
			verifyItem.Name,
		)
	}

	// --- Delete ---
	t.Log("Step 4: Delete item")
	status, body = doRequest(
		t, client, http.MethodDelete, itemURL, nil, headers,
	)

	if status != http.StatusNoContent {
		t.Fatalf("Delete: expected 204, got %d. Body: %s",
			status, body)
	}

	// Verify deletion
	status, _ = doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	if status != http.StatusNotFound {
		t.Errorf("Delete verify: expected 404, got %d", status)
	}

	t.Log("CRUD operations completed successfully")
}

// TestIntegration_APIKeyAuthentication tests API key authentication
// when an API key is configured via environment.
func TestIntegration_APIKeyAuthentication(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv(EnvAPIKey)
	if apiKey == "" {
		t.Skip("INTEGRATION_API_KEY not set, skipping API key test")
	}

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()

	// Request with valid API key should succeed.
	headers := map[string]string{
		"X-API-Key":    apiKey,
		"Content-Type": "application/json",
	}

	status, body := doRequest(
		t, client, http.MethodGet,
		base+"/api/v1/items", nil, headers,
	)

	if status != http.StatusOK {
		t.Errorf(
			"Expected 200 with valid API key, got %d. Body: %s",
			status, body,
		)
	}

	// Request with invalid API key should fail.
	badHeaders := map[string]string{
		"X-API-Key": "invalid-key-12345",
	}

	status, _ = doRequest(
		t, client, http.MethodGet,
		base+"/api/v1/items", nil, badHeaders,
	)

	if status != http.StatusUnauthorized {
		t.Errorf(
			"Expected 401 with invalid API key, got %d", status,
		)
	}

	t.Log("API key authentication test passed")
}

// TestIntegration_BasicAuthentication tests HTTP Basic authentication
// when credentials are configured via environment.
func TestIntegration_BasicAuthentication(t *testing.T) {
	t.Parallel()

	user := os.Getenv(EnvBasicUser)
	pass := os.Getenv(EnvBasicPass)
	if user == "" || pass == "" {
		t.Skip(
			"INTEGRATION_BASIC_USER/PASS not set, skipping basic auth",
		)
	}

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()

	// Build request with Basic auth.
	req, err := http.NewRequest(
		http.MethodGet, base+"/api/v1/items", nil,
	)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf(
			"Expected 200 with valid credentials, got %d",
			resp.StatusCode,
		)
	}

	// Request with wrong password should fail.
	reqBad, _ := http.NewRequest(
		http.MethodGet, base+"/api/v1/items", nil,
	)
	reqBad.SetBasicAuth(user, "wrong-password")

	respBad, err := client.Do(reqBad)
	if err != nil {
		t.Fatalf("Bad auth request failed: %v", err)
	}
	respBad.Body.Close()

	if respBad.StatusCode != http.StatusUnauthorized {
		t.Errorf(
			"Expected 401 with wrong password, got %d",
			respBad.StatusCode,
		)
	}

	t.Log("Basic authentication test passed")
}

// TestIntegration_UnauthorizedAccess verifies that protected endpoints
// return 401 when no authentication is provided and the server has
// auth enabled.
func TestIntegration_UnauthorizedAccess(t *testing.T) {
	t.Parallel()

	// This test only makes sense when auth is configured.
	apiKey := os.Getenv(EnvAPIKey)
	user := os.Getenv(EnvBasicUser)
	if apiKey == "" && user == "" {
		t.Skip(
			"No auth configured, skipping unauthorized access test",
		)
	}

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()

	endpoints := []string{
		"/api/v1/items",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			status, body := doRequest(
				t, client, http.MethodGet,
				base+ep, nil, nil,
			)

			if status != http.StatusUnauthorized {
				t.Errorf(
					"Expected 401 for %s without auth, got %d. Body: %s",
					ep, status, body,
				)
			}
		})
	}

	t.Log("Unauthorized access test passed")
}

// TestIntegration_MTLSAuthentication tests mutual TLS authentication
// when certificate paths are configured via environment.
func TestIntegration_MTLSAuthentication(t *testing.T) {
	t.Parallel()

	caCert := os.Getenv(EnvCACertPath)
	clientCert := os.Getenv(EnvClientCert)
	clientKey := os.Getenv(EnvClientKey)

	if caCert == "" || clientCert == "" || clientKey == "" {
		t.Skip("mTLS cert paths not set, skipping mTLS test")
	}

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client, err := createTLSClient(caCert, clientCert, clientKey)
	if err != nil {
		t.Fatalf("Failed to create TLS client: %v", err)
	}

	status, body := doRequest(
		t, client, http.MethodGet,
		base+"/api/v1/items", nil, nil,
	)

	if status != http.StatusOK {
		t.Errorf(
			"Expected 200 with valid client cert, got %d. Body: %s",
			status, body,
		)
	}

	t.Log("mTLS authentication test passed")
}

// TestIntegration_OIDCAuthentication tests OIDC/JWT bearer token
// authentication when Keycloak is available.
func TestIntegration_OIDCAuthentication(t *testing.T) {
	t.Parallel()

	keycloakURL := getEnvOrDefault(
		EnvKeycloakURL, DefaultKeycloakURL,
	)

	// Check if Keycloak is reachable.
	skipIfServiceUnavailable(t, keycloakURL)

	clientID := os.Getenv("INTEGRATION_OIDC_CLIENT_ID")
	clientSecret := os.Getenv("INTEGRATION_OIDC_CLIENT_SECRET")
	realm := os.Getenv("INTEGRATION_OIDC_REALM")

	if clientID == "" || clientSecret == "" || realm == "" {
		t.Skip("OIDC client config not set, skipping OIDC test")
	}

	token, err := getKeycloakToken(
		keycloakURL, realm, clientID, clientSecret,
	)
	if err != nil {
		t.Fatalf("Failed to obtain Keycloak token: %v", err)
	}

	base := serverURL()
	client := createHTTPClient()

	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}

	status, body := doRequest(
		t, client, http.MethodGet,
		base+"/api/v1/items", nil, headers,
	)

	if status != http.StatusOK {
		t.Errorf(
			"Expected 200 with valid OIDC token, got %d. Body: %s",
			status, body,
		)
	}

	t.Log("OIDC authentication test passed")
}

// TestIntegration_OIDCPasswordGrant tests OIDC authentication using
// password grant tokens from Keycloak.
func TestIntegration_OIDCPasswordGrant(t *testing.T) {
	t.Parallel()

	keycloakURL := getEnvOrDefault(
		EnvKeycloakURL, DefaultKeycloakURL,
	)

	skipIfServiceUnavailable(t, keycloakURL)

	clientID := os.Getenv("INTEGRATION_OIDC_CLIENT_ID")
	clientSecret := os.Getenv("INTEGRATION_OIDC_CLIENT_SECRET")
	realm := os.Getenv("INTEGRATION_OIDC_REALM")
	username := os.Getenv("INTEGRATION_OIDC_USERNAME")
	password := os.Getenv("INTEGRATION_OIDC_PASSWORD")

	if clientID == "" || clientSecret == "" || realm == "" {
		t.Skip("OIDC client config not set, skipping")
	}
	if username == "" || password == "" {
		t.Skip(
			"OIDC username/password not set, skipping password grant",
		)
	}

	token, err := getKeycloakPasswordToken(
		keycloakURL, realm, clientID, clientSecret,
		username, password,
	)
	if err != nil {
		t.Fatalf("Failed to obtain password grant token: %v", err)
	}

	if token == "" {
		t.Fatal("Received empty token from password grant")
	}

	base := serverURL()
	client := createHTTPClient()

	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}

	status, body := doRequest(
		t, client, http.MethodGet,
		base+"/api/v1/items", nil, headers,
	)

	if status != http.StatusOK {
		t.Errorf(
			"Expected 200 with password grant token, got %d. Body: %s",
			status, body,
		)
	}

	t.Log("OIDC password grant authentication test passed")
}

// TestIntegration_MultiAuthMethods tests that the server accepts
// multiple authentication methods when configured in multi mode.
func TestIntegration_MultiAuthMethods(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv(EnvAPIKey)
	user := os.Getenv(EnvBasicUser)
	pass := os.Getenv(EnvBasicPass)

	if apiKey == "" && (user == "" || pass == "") {
		t.Skip(
			"Neither API key nor basic auth configured, " +
				"skipping multi-auth test",
		)
	}

	base := serverURL()
	skipIfServiceUnavailable(t, base+"/health")

	client := createHTTPClient()

	// Sub-test: API key authentication.
	if apiKey != "" {
		t.Run("api_key", func(t *testing.T) {
			headers := map[string]string{
				"X-API-Key": apiKey,
			}

			status, body := doRequest(
				t, client, http.MethodGet,
				base+"/api/v1/items", nil, headers,
			)

			if status != http.StatusOK {
				t.Errorf(
					"Expected 200 with API key, got %d. Body: %s",
					status, body,
				)
			}
		})
	}

	// Sub-test: Basic authentication.
	if user != "" && pass != "" {
		t.Run("basic_auth", func(t *testing.T) {
			creds := base64.StdEncoding.EncodeToString(
				[]byte(user + ":" + pass),
			)
			headers := map[string]string{
				"Authorization": "Basic " + creds,
			}

			status, body := doRequest(
				t, client, http.MethodGet,
				base+"/api/v1/items", nil, headers,
			)

			if status != http.StatusOK {
				t.Errorf(
					"Expected 200 with basic auth, got %d. Body: %s",
					status, body,
				)
			}
		})
	}

	// Sub-test: No authentication should fail.
	t.Run("no_auth", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet,
			base+"/api/v1/items", nil, nil,
		)

		if status != http.StatusUnauthorized {
			t.Errorf(
				"Expected 401 without auth, got %d. Body: %s",
				status, body,
			)
		}
	})

	t.Log("Multi-auth methods test passed")
}

// TestIntegration_MTLSWithTLSClient tests mTLS authentication using
// a TLS-aware client and the TLS service availability check.
func TestIntegration_MTLSWithTLSClient(t *testing.T) {
	t.Parallel()

	caCert := os.Getenv(EnvCACertPath)
	clientCert := os.Getenv(EnvClientCert)
	clientKey := os.Getenv(EnvClientKey)

	if caCert == "" || clientCert == "" || clientKey == "" {
		t.Skip("mTLS cert paths not set, skipping")
	}

	base := serverURL()
	skipIfServiceUnavailableTLS(
		t, base+"/health", caCert, clientCert, clientKey,
	)

	client, err := createTLSClient(caCert, clientCert, clientKey)
	if err != nil {
		t.Fatalf("Failed to create TLS client: %v", err)
	}

	// Verify list items works with mTLS.
	status, body := doRequest(
		t, client, http.MethodGet,
		base+"/api/v1/items", nil, nil,
	)

	if status != http.StatusOK {
		t.Errorf(
			"Expected 200 with mTLS client cert, got %d. Body: %s",
			status, body,
		)
	}

	// Verify CRUD create works with mTLS.
	createBody := []byte(
		`{"name":"mTLS Test Item","price":10.00}`,
	)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	status, body = doRequest(
		t, client, http.MethodPost,
		base+"/api/v1/items",
		bytes.NewReader(createBody), headers,
	)

	if status != http.StatusCreated {
		t.Errorf(
			"Expected 201 for mTLS create, got %d. Body: %s",
			status, body,
		)
	}

	// Clean up: parse created item and delete it.
	var createResp apiResponse
	if err := json.Unmarshal(body, &createResp); err == nil {
		var created itemResponse
		if err := json.Unmarshal(
			createResp.Data, &created,
		); err == nil && created.ID != "" {
			itemURL := fmt.Sprintf(
				"%s/api/v1/items/%s", base, created.ID,
			)
			doRequest(
				t, client, http.MethodDelete,
				itemURL, nil, nil,
			)
		}
	}

	t.Log("mTLS with TLS client test passed")
}

// buildAuthHeaders returns a header map populated with authentication
// credentials from environment variables, if available.
func buildAuthHeaders(t *testing.T) map[string]string {
	t.Helper()

	headers := make(map[string]string)

	if apiKey := os.Getenv(EnvAPIKey); apiKey != "" {
		headers["X-API-Key"] = apiKey
		return headers
	}

	user := os.Getenv(EnvBasicUser)
	pass := os.Getenv(EnvBasicPass)
	if user != "" && pass != "" {
		creds := base64.StdEncoding.EncodeToString(
			[]byte(user + ":" + pass),
		)
		headers["Authorization"] = "Basic " + creds
	}

	return headers
}
