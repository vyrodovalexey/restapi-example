//go:build e2e

package e2e_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/config"
)

// ----------------------------------------------------------------------------
// Auth-mode server builders.
// ----------------------------------------------------------------------------

func startAPIKeyServer(t *testing.T) *e2eServer {
	t.Helper()
	a, err := auth.NewAPIKeyAuthenticator(apiKeyConfig())
	if err != nil {
		t.Fatalf("Failed to create API key authenticator: %v", err)
	}
	return startServer(t, &config.Config{
		MetricsEnabled: true,
		AuthMode:       "apikey",
		APIKeys:        apiKeyConfig(),
	}, a)
}

func startBasicServer(t *testing.T) *e2eServer {
	t.Helper()
	users := basicAuthConfig(t)
	a, err := auth.NewBasicAuthenticator(users)
	if err != nil {
		t.Fatalf("Failed to create basic authenticator: %v", err)
	}
	return startServer(t, &config.Config{
		MetricsEnabled: true,
		AuthMode:       "basic",
		BasicAuthUsers: users,
	}, a)
}

func startOIDCServer(t *testing.T) *e2eServer {
	t.Helper()
	verifier, err := auth.NewOIDCTokenVerifier(oidcIssuerURL())
	if err != nil {
		t.Skipf("OIDC verifier init failed (Keycloak unavailable?): %v", err)
	}
	t.Cleanup(verifier.Stop)
	a := auth.NewOIDCAuthenticator(verifier, "")
	return startServer(t, &config.Config{
		MetricsEnabled: true,
		AuthMode:       "oidc",
		OIDCIssuerURL:  oidcIssuerURL(),
		OIDCClientID:   oidcClientID(),
	}, a)
}

func startMTLSServer(t *testing.T) *e2eServer {
	t.Helper()
	skipIfCertsUnavailable(t)
	return startServer(t, &config.Config{
		MetricsEnabled: true,
		AuthMode:       "mtls",
		TLSEnabled:     true,
		TLSCertPath:    serverCertPath(),
		TLSKeyPath:     serverKeyPath(),
		TLSCAPath:      caCertPath(),
		TLSClientAuth:  "require",
	}, auth.NewMTLSAuthenticator())
}

func apiKeyHeaders() map[string]string {
	return map[string]string{auth.APIKeyHeader: apiKeyValue()}
}

func basicAuthHeaders() map[string]string {
	// Use SetBasicAuth-equivalent header construction.
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth(basicUser(), basicPass())
	return map[string]string{"Authorization": req.Header.Get("Authorization")}
}

func bearerHeaders(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// ----------------------------------------------------------------------------
// E2E user journeys.
// ----------------------------------------------------------------------------

// TestE2E_APIKeyWorkflow drives the complete CRUD journey using API key auth.
func TestE2E_APIKeyWorkflow(t *testing.T) {
	ts := startAPIKeyServer(t)
	client := newHTTPClient()
	fullCRUDJourney(t, client, ts.baseURL, jsonHeaders(apiKeyHeaders()))
	t.Log("API key workflow completed successfully")
}

// TestE2E_BasicAuthWorkflow drives the complete CRUD journey using basic auth.
func TestE2E_BasicAuthWorkflow(t *testing.T) {
	ts := startBasicServer(t)
	client := newHTTPClient()
	fullCRUDJourney(t, client, ts.baseURL, jsonHeaders(basicAuthHeaders()))
	t.Log("Basic auth workflow completed successfully")
}

// TestE2E_OIDCWorkflow obtains a Keycloak password-grant token and drives the
// complete CRUD journey through the OIDC-protected server.
func TestE2E_OIDCWorkflow(t *testing.T) {
	skipIfKeycloakUnavailable(t)

	t.Log("Step 1: Obtain OIDC token from Keycloak")
	token, err := getKeycloakPasswordToken(
		oidcClientID(), oidcClientSecret(),
		oidcUsername(), oidcPassword(),
	)
	if err != nil {
		t.Fatalf("Failed to obtain Keycloak token: %v", err)
	}
	if token == "" {
		t.Fatal("Received empty token from Keycloak")
	}

	ts := startOIDCServer(t)
	client := newHTTPClient()
	fullCRUDJourney(t, client, ts.baseURL, jsonHeaders(bearerHeaders(token)))
	t.Log("OIDC workflow completed successfully")
}

// TestE2E_MTLSWorkflow drives the complete CRUD journey over a mutually
// authenticated TLS channel using the Vault-issued client certificate.
func TestE2E_MTLSWorkflow(t *testing.T) {
	ts := startMTLSServer(t)

	client, err := newTLSClient(caCertPath(), clientCertPath(), clientKeyPath())
	if err != nil {
		t.Fatalf("Failed to create TLS client: %v", err)
	}

	t.Log("Running full CRUD journey over mTLS")
	fullCRUDJourney(t, client, ts.baseURL, map[string]string{
		"Content-Type": "application/json",
	})
	t.Log("mTLS workflow completed successfully")
}

// TestE2E_PublicEndpointsAlwaysAccessible verifies health, ready and metrics
// are reachable without authentication on an auth-enabled server.
func TestE2E_PublicEndpointsAlwaysAccessible(t *testing.T) {
	ts := startAPIKeyServer(t)
	client := newHTTPClient()

	for _, ep := range []string{"/health", "/ready", "/metrics"} {
		t.Run(ep, func(t *testing.T) {
			status, body := doRequest(
				t, client, http.MethodGet, ts.baseURL+ep, nil, nil,
			)
			if status != http.StatusOK {
				t.Errorf("Expected 200 for %s, got %d. Body: %s",
					ep, status, body)
			}
		})
	}
}

// TestE2E_UnauthorizedAccessDenied verifies protected endpoints return 401
// without credentials across all methods.
func TestE2E_UnauthorizedAccessDenied(t *testing.T) {
	ts := startAPIKeyServer(t)
	client := newHTTPClient()

	protected := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/items"},
		{http.MethodPost, "/api/v1/items"},
		{http.MethodGet, "/api/v1/items/nonexistent"},
		{http.MethodPut, "/api/v1/items/nonexistent"},
		{http.MethodDelete, "/api/v1/items/nonexistent"},
		{http.MethodPost, "/graphql"},
	}

	for _, ep := range protected {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			status, body := doRequest(
				t, client, ep.method, ts.baseURL+ep.path, nil,
				map[string]string{"Content-Type": "application/json"},
			)
			if status != http.StatusUnauthorized {
				t.Errorf("Expected 401 for %s %s, got %d. Body: %s",
					ep.method, ep.path, status, body)
			}
		})
	}
}

// TestE2E_GracefulDegradation verifies the server handles invalid auth without
// crashing and stays healthy, with /metrics still serving the metric set.
func TestE2E_GracefulDegradation(t *testing.T) {
	ts := startAPIKeyServer(t)
	client := newHTTPClient()

	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"empty_api_key", map[string]string{auth.APIKeyHeader: ""}},
		{"invalid_api_key", map[string]string{auth.APIKeyHeader: "bad"}},
		{"malformed_bearer", map[string]string{"Authorization": "Bearer not.a.jwt"}},
		{"invalid_basic", map[string]string{"Authorization": "Basic bm90OnZhbGlk"}},
		{"garbage_auth", map[string]string{"Authorization": "Garbage xyz"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := doRequest(
				t, client, http.MethodGet,
				ts.baseURL+"/api/v1/items", nil, tc.headers,
			)
			if status != http.StatusUnauthorized {
				t.Errorf("Expected 401, got %d", status)
			}
			// Server must remain healthy.
			hs, _ := doRequest(
				t, client, http.MethodGet, ts.baseURL+"/health", nil, nil,
			)
			if hs != http.StatusOK {
				t.Errorf("Server unhealthy after bad auth: %d", hs)
			}
		})
	}

	// Metrics endpoint still serves a valid exposition.
	status, body := doRequest(
		t, client, http.MethodGet, ts.baseURL+"/metrics", nil, nil,
	)
	if status != http.StatusOK {
		t.Fatalf("metrics: expected 200, got %d", status)
	}
	if !strings.Contains(string(body), "# HELP") {
		t.Error("metrics output not in Prometheus exposition format")
	}
	if !strings.Contains(string(body), "auth_attempts_total") {
		t.Error("metrics output missing auth_attempts_total after failed auth")
	}
}

// TestE2E_ConcurrentRequests verifies the server handles concurrent
// authenticated creates correctly, then cleans up.
func TestE2E_ConcurrentRequests(t *testing.T) {
	ts := startAPIKeyServer(t)
	client := newHTTPClient()
	headers := jsonHeaders(apiKeyHeaders())

	const numConcurrent = 10
	type result struct {
		status int
		itemID string
	}
	results := make(chan result, numConcurrent)

	for i := range numConcurrent {
		go func(idx int) {
			created := createItem(t, client, ts.baseURL, headers, createItemRequest{
				Name:  itemName(idx),
				Price: float64(idx) * 10.0,
			})
			results <- result{status: http.StatusCreated, itemID: created.ID}
		}(i)
	}

	var ids []string
	for range numConcurrent {
		r := <-results
		if r.status == http.StatusCreated && r.itemID != "" {
			ids = append(ids, r.itemID)
		}
	}

	if len(ids) != numConcurrent {
		t.Errorf("Expected %d created items, got %d", numConcurrent, len(ids))
	}

	for _, id := range ids {
		deleteItem(t, client, ts.baseURL, id, headers)
	}
}

// itemName builds a deterministic-ish item name for concurrency tests.
func itemName(idx int) string {
	return "Concurrent Item " + string(rune('A'+idx%26))
}
