//go:build integration

package integration_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/config"
)

// ----------------------------------------------------------------------------
// Server harness builders (one in-process server per auth mode).
// ----------------------------------------------------------------------------

// startNoneServer starts a server with no authentication.
func startNoneServer(t *testing.T) *testServer {
	t.Helper()
	return startServer(t, &config.Config{
		MetricsEnabled: true,
		AuthMode:       "none",
	}, nil)
}

// startAPIKeyServer starts a server with API key authentication.
func startAPIKeyServer(t *testing.T) *testServer {
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

// startBasicServer starts a server with HTTP Basic authentication.
func startBasicServer(t *testing.T) *testServer {
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

// startOIDCServer starts a server with OIDC authentication backed by the live
// Keycloak. The verifier uses the HOST issuer so the issuer check matches
// tokens minted by Keycloak's host-facing token endpoint.
func startOIDCServer(t *testing.T) *testServer {
	t.Helper()

	verifier, err := auth.NewOIDCTokenVerifier(oidcIssuerURL())
	if err != nil {
		t.Skipf("OIDC verifier init failed (Keycloak unavailable?): %v", err)
	}
	t.Cleanup(verifier.Stop)

	// No audience restriction: Keycloak access tokens carry the "account"
	// audience by default rather than the client id.
	a := auth.NewOIDCAuthenticator(verifier, "")

	return startServer(t, &config.Config{
		MetricsEnabled: true,
		AuthMode:       "oidc",
		OIDCIssuerURL:  oidcIssuerURL(),
		OIDCClientID:   oidcClientID(),
	}, a)
}

// startMultiServer starts a server in multi-auth mode accepting apikey, basic
// and OIDC (when Keycloak is reachable).
func startMultiServer(t *testing.T, withOIDC bool) *testServer {
	t.Helper()

	apiKeyAuth, err := auth.NewAPIKeyAuthenticator(apiKeyConfig())
	if err != nil {
		t.Fatalf("Failed to create API key authenticator: %v", err)
	}

	users := basicAuthConfig(t)
	basicAuth, err := auth.NewBasicAuthenticator(users)
	if err != nil {
		t.Fatalf("Failed to create basic authenticator: %v", err)
	}

	authenticators := []auth.Authenticator{apiKeyAuth, basicAuth}

	cfg := &config.Config{
		MetricsEnabled: true,
		AuthMode:       "multi",
		APIKeys:        apiKeyConfig(),
		BasicAuthUsers: users,
	}

	if withOIDC {
		verifier, verr := auth.NewOIDCTokenVerifier(oidcIssuerURL())
		if verr != nil {
			t.Skipf("OIDC verifier init failed: %v", verr)
		}
		t.Cleanup(verifier.Stop)
		authenticators = append(
			authenticators, auth.NewOIDCAuthenticator(verifier, ""),
		)
		cfg.OIDCIssuerURL = oidcIssuerURL()
		cfg.OIDCClientID = oidcClientID()
	}

	return startServer(t, cfg, auth.NewMultiAuthenticator(authenticators...))
}

// startMTLSServer starts a TLS server with client-auth=require using the
// Vault-issued server cert + CA. The mTLS authenticator validates the
// presented client certificate.
func startMTLSServer(t *testing.T) *testServer {
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

// ----------------------------------------------------------------------------
// Common header helpers.
// ----------------------------------------------------------------------------

func apiKeyHeaders() map[string]string {
	return map[string]string{auth.APIKeyHeader: apiKeyValue()}
}

func basicAuthHeaders(user, pass string) map[string]string {
	creds := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	return map[string]string{"Authorization": "Basic " + creds}
}

func bearerHeaders(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// ----------------------------------------------------------------------------
// Probe endpoints (auth-independent) — exercised against a multi-auth server.
// ----------------------------------------------------------------------------

// TestIntegration_ProbeEndpoints verifies health, ready and metrics endpoints
// are reachable without authentication on a server with auth enabled, and that
// the extended metric set is exposed once the relevant paths are exercised.
func TestIntegration_ProbeEndpoints(t *testing.T) {
	ts := startAPIKeyServer(t)
	client := newHTTPClient()

	// Exercise auth (success + failure) and a store operation so that the
	// labelled counters (auth_attempts_total, store_operations_total) are
	// emitted in the exposition (Prometheus omits unobserved label sets).
	doRequest(t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil, apiKeyHeaders())
	doRequest(t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
		map[string]string{auth.APIKeyHeader: "bogus"})

	t.Run("health", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/health", nil, nil,
		)
		if status != http.StatusOK {
			t.Fatalf("Expected 200, got %d. Body: %s", status, body)
		}
		var resp apiResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		var health healthResponse
		if err := json.Unmarshal(resp.Data, &health); err != nil {
			t.Fatalf("Failed to parse health data: %v", err)
		}
		if health.Status != "healthy" {
			t.Errorf("Expected 'healthy', got %q", health.Status)
		}
	})

	t.Run("ready", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/ready", nil, nil,
		)
		if status != http.StatusOK {
			t.Fatalf("Expected 200, got %d. Body: %s", status, body)
		}
		var resp apiResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}
		var ready readyResponse
		if err := json.Unmarshal(resp.Data, &ready); err != nil {
			t.Fatalf("Failed to parse ready data: %v", err)
		}
		if ready.Status != "ready" {
			t.Errorf("Expected 'ready', got %q", ready.Status)
		}
	})

	t.Run("metrics", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/metrics", nil, nil,
		)
		if status != http.StatusOK {
			t.Fatalf("Expected 200, got %d", status)
		}
		// Verify the new metric set is exposed on a live server.
		for _, name := range []string{
			"http_requests_total",
			"http_request_duration_seconds",
			"http_requests_in_flight",
			"auth_attempts_total",
			"store_operations_total",
			"store_operation_duration_seconds",
			"http_response_size_bytes",
			"build_info",
			"go_goroutines",
			"process_cpu_seconds_total",
		} {
			if !strings.Contains(string(body), name) {
				t.Errorf("metrics output missing %q", name)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// AM-NONE: no auth mode.
// ----------------------------------------------------------------------------

// TestIntegration_NoneMode_CRUD verifies that without auth the full CRUD
// lifecycle works (AM-NONE-1) and validation errors map correctly (AM-NONE-2).
func TestIntegration_NoneMode_CRUD(t *testing.T) {
	ts := startNoneServer(t)
	client := newHTTPClient()
	runCRUDLifecycle(t, client, ts.baseURL, nil)

	// Negative: invalid body yields 400, not 401.
	status, _ := doRequest(
		t, client, http.MethodPost, ts.baseURL+"/api/v1/items",
		bytes.NewReader([]byte("not json")),
		map[string]string{"Content-Type": "application/json"},
	)
	if status != http.StatusBadRequest {
		t.Errorf("AM-NONE-2: expected 400 for invalid body, got %d", status)
	}
}

// ----------------------------------------------------------------------------
// AM-KEY: API key mode (positive + negative).
// ----------------------------------------------------------------------------

// TestIntegration_APIKeyMode covers AM-KEY-1/2/3 across REST and GraphQL.
func TestIntegration_APIKeyMode(t *testing.T) {
	ts := startAPIKeyServer(t)
	client := newHTTPClient()

	t.Run("valid_key_rest", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet,
			ts.baseURL+"/api/v1/items", nil, apiKeyHeaders(),
		)
		if status != http.StatusOK {
			t.Errorf("AM-KEY-1: expected 200, got %d. Body: %s", status, body)
		}
	})

	t.Run("valid_key_graphql", func(t *testing.T) {
		runGraphQLQuery(t, client, ts.baseURL, apiKeyHeaders())
	})

	t.Run("invalid_key", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			map[string]string{auth.APIKeyHeader: "bogus-key"},
		)
		if status != http.StatusUnauthorized {
			t.Errorf("AM-KEY-2: expected 401, got %d", status)
		}
	})

	t.Run("missing_key", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil, nil,
		)
		if status != http.StatusUnauthorized {
			t.Errorf("AM-KEY-3: expected 401, got %d", status)
		}
	})

	t.Run("crud_lifecycle", func(t *testing.T) {
		runCRUDLifecycle(t, client, ts.baseURL, apiKeyHeaders())
	})
}

// ----------------------------------------------------------------------------
// AM-BASIC: basic auth mode (positive + negative).
// ----------------------------------------------------------------------------

// TestIntegration_BasicMode covers AM-BASIC-1/2/3.
func TestIntegration_BasicMode(t *testing.T) {
	ts := startBasicServer(t)
	client := newHTTPClient()

	t.Run("valid_credentials", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			basicAuthHeaders(basicUser(), basicPass()),
		)
		if status != http.StatusOK {
			t.Errorf("AM-BASIC-1: expected 200, got %d. Body: %s", status, body)
		}
	})

	t.Run("wrong_password", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			basicAuthHeaders(basicUser(), "wrong-password"),
		)
		if status != http.StatusUnauthorized {
			t.Errorf("AM-BASIC-2: expected 401, got %d", status)
		}
	})

	t.Run("no_credentials", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil, nil,
		)
		if status != http.StatusUnauthorized {
			t.Errorf("AM-BASIC-3: expected 401, got %d", status)
		}
	})

	t.Run("crud_lifecycle", func(t *testing.T) {
		runCRUDLifecycle(
			t, client, ts.baseURL,
			basicAuthHeaders(basicUser(), basicPass()),
		)
	})
}

// ----------------------------------------------------------------------------
// AM-OIDC: OIDC mode against the live Keycloak (password grant).
// ----------------------------------------------------------------------------

// TestIntegration_OIDCMode_PasswordGrant covers AM-OIDC-1/2/3 for the
// standard test-user using a Keycloak password-grant token.
func TestIntegration_OIDCMode_PasswordGrant(t *testing.T) {
	skipIfKeycloakUnavailable(t)

	token, err := getKeycloakPasswordToken(
		oidcClientID(), oidcClientSecret(),
		oidcUsername(), oidcPassword(),
	)
	if err != nil {
		t.Fatalf("Failed to obtain Keycloak token: %v", err)
	}

	ts := startOIDCServer(t)
	client := newHTTPClient()

	t.Run("valid_token_rest", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			bearerHeaders(token),
		)
		if status != http.StatusOK {
			t.Errorf("AM-OIDC-1: expected 200, got %d. Body: %s", status, body)
		}
	})

	t.Run("valid_token_graphql", func(t *testing.T) {
		runGraphQLQuery(t, client, ts.baseURL, bearerHeaders(token))
	})

	t.Run("invalid_token", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			bearerHeaders("not.a.valid.jwt"),
		)
		if status != http.StatusUnauthorized {
			t.Errorf("AM-OIDC-2: expected 401, got %d", status)
		}
	})

	t.Run("missing_bearer", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			map[string]string{"Authorization": "Token abc"},
		)
		if status != http.StatusUnauthorized {
			t.Errorf("AM-OIDC-3: expected 401, got %d", status)
		}
	})

	t.Run("crud_lifecycle", func(t *testing.T) {
		runCRUDLifecycle(t, client, ts.baseURL, bearerHeaders(token))
	})
}

// TestIntegration_OIDCMode_AdminUser verifies the admin-user token is also
// accepted by the OIDC-protected server.
func TestIntegration_OIDCMode_AdminUser(t *testing.T) {
	skipIfKeycloakUnavailable(t)

	adminUser := getEnvOrDefault(EnvOIDCAdminUser, "admin-user")
	adminPass := getEnvOrDefault(EnvOIDCAdminPass, "admin-password")

	token, err := getKeycloakPasswordToken(
		oidcClientID(), oidcClientSecret(), adminUser, adminPass,
	)
	if err != nil {
		t.Fatalf("Failed to obtain admin Keycloak token: %v", err)
	}

	ts := startOIDCServer(t)
	client := newHTTPClient()

	status, body := doRequest(
		t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
		bearerHeaders(token),
	)
	if status != http.StatusOK {
		t.Errorf("admin-user OIDC: expected 200, got %d. Body: %s", status, body)
	}
}

// ----------------------------------------------------------------------------
// AM-MULTI: multi-auth mode accepts multiple methods (AM-MULTI-1/2/3).
// ----------------------------------------------------------------------------

// TestIntegration_MultiMode verifies that multiple methods are accepted by a
// single multi-auth server instance, and that no/invalid credentials fail.
func TestIntegration_MultiMode(t *testing.T) {
	kcUp := keycloakReachable()
	ts := startMultiServer(t, kcUp)
	client := newHTTPClient()

	t.Run("api_key", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			apiKeyHeaders(),
		)
		if status != http.StatusOK {
			t.Errorf("AM-MULTI(apikey): expected 200, got %d", status)
		}
	})

	t.Run("basic_auth", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
			basicAuthHeaders(basicUser(), basicPass()),
		)
		if status != http.StatusOK {
			t.Errorf("AM-MULTI(basic): expected 200, got %d", status)
		}
	})

	if kcUp {
		t.Run("oidc", func(t *testing.T) {
			token, err := getKeycloakPasswordToken(
				oidcClientID(), oidcClientSecret(),
				oidcUsername(), oidcPassword(),
			)
			if err != nil {
				t.Fatalf("Failed to obtain token: %v", err)
			}
			status, _ := doRequest(
				t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil,
				bearerHeaders(token),
			)
			if status != http.StatusOK {
				t.Errorf("AM-MULTI(oidc): expected 200, got %d", status)
			}
		})
	}

	t.Run("no_auth", func(t *testing.T) {
		status, _ := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil, nil,
		)
		if status != http.StatusUnauthorized {
			t.Errorf("AM-MULTI-2: expected 401, got %d", status)
		}
	})
}

// ----------------------------------------------------------------------------
// AM-MTLS: mutual TLS using Vault-issued client cert (AM-MTLS-1/2/3).
// ----------------------------------------------------------------------------

// TestIntegration_MTLSMode covers AM-MTLS-1 (valid cert) and AM-MTLS-2/3
// (handshake rejection without / with an untrusted cert).
func TestIntegration_MTLSMode(t *testing.T) {
	ts := startMTLSServer(t)

	t.Run("valid_client_cert", func(t *testing.T) {
		client, err := newTLSClient(
			caCertPath(), clientCertPath(), clientKeyPath(),
		)
		if err != nil {
			t.Fatalf("Failed to build TLS client: %v", err)
		}
		status, body := doRequest(
			t, client, http.MethodGet, ts.baseURL+"/api/v1/items", nil, nil,
		)
		if status != http.StatusOK {
			t.Errorf("AM-MTLS-1: expected 200, got %d. Body: %s", status, body)
		}
		// Exercise CRUD over the mTLS channel.
		runCRUDLifecycle(t, client, ts.baseURL, nil)
	})

	t.Run("no_client_cert_rejected", func(t *testing.T) {
		// A client trusting the CA but presenting no client cert must be
		// rejected at the TLS handshake (require client auth).
		noCertClient := newNoCertTLSClient(t)
		_, err := noCertClient.Get(ts.baseURL + "/api/v1/items")
		if err == nil {
			t.Errorf("AM-MTLS-2: expected handshake failure without client cert")
		}
	})
}

// ----------------------------------------------------------------------------
// Shared CRUD + GraphQL drivers.
// ----------------------------------------------------------------------------

// runCRUDLifecycle drives create → read → update → verify → delete → verify
// against the REST API using the provided authentication headers.
func runCRUDLifecycle(
	t *testing.T,
	client *http.Client,
	base string,
	authHeaders map[string]string,
) {
	t.Helper()

	headers := map[string]string{"Content-Type": "application/json"}
	for k, v := range authHeaders {
		headers[k] = v
	}

	// Create.
	createBody, _ := json.Marshal(map[string]any{
		"name":        "Integration CRUD Item",
		"description": "created by integration test",
		"price":       42.0,
	})
	status, body := doRequest(
		t, client, http.MethodPost, base+"/api/v1/items",
		bytes.NewReader(createBody), headers,
	)
	if status != http.StatusCreated {
		t.Fatalf("Create: expected 201, got %d. Body: %s", status, body)
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

	itemURL := fmt.Sprintf("%s/api/v1/items/%s", base, created.ID)

	// Read.
	status, body = doRequest(t, client, http.MethodGet, itemURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("Read: expected 200, got %d. Body: %s", status, body)
	}

	// Update.
	updateBody, _ := json.Marshal(map[string]any{
		"name":        "Integration CRUD Item Updated",
		"description": "updated",
		"price":       84.0,
	})
	status, body = doRequest(
		t, client, http.MethodPut, itemURL,
		bytes.NewReader(updateBody), headers,
	)
	if status != http.StatusOK {
		t.Fatalf("Update: expected 200, got %d. Body: %s", status, body)
	}

	// Verify update.
	status, body = doRequest(t, client, http.MethodGet, itemURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("Verify: expected 200, got %d", status)
	}
	var verifyResp apiResponse
	_ = json.Unmarshal(body, &verifyResp)
	var verifyItem itemResponse
	_ = json.Unmarshal(verifyResp.Data, &verifyItem)
	if verifyItem.Name != "Integration CRUD Item Updated" {
		t.Errorf("Verify: unexpected name %q", verifyItem.Name)
	}

	// Delete.
	status, _ = doRequest(t, client, http.MethodDelete, itemURL, nil, headers)
	if status != http.StatusNoContent {
		t.Fatalf("Delete: expected 204, got %d", status)
	}

	// Verify delete.
	status, _ = doRequest(t, client, http.MethodGet, itemURL, nil, headers)
	if status != http.StatusNotFound {
		t.Errorf("Verify delete: expected 404, got %d", status)
	}
}

// runGraphQLQuery issues a basic GraphQL items query and asserts a 200 with no
// GraphQL errors.
func runGraphQLQuery(
	t *testing.T,
	client *http.Client,
	base string,
	authHeaders map[string]string,
) {
	t.Helper()

	headers := map[string]string{"Content-Type": "application/json"}
	for k, v := range authHeaders {
		headers[k] = v
	}

	payload, _ := json.Marshal(map[string]string{
		"query": "{ items { id name price } }",
	})
	status, body := doRequest(
		t, client, http.MethodPost, base+"/graphql",
		bytes.NewReader(payload), headers,
	)
	if status != http.StatusOK {
		t.Fatalf("GraphQL: expected 200, got %d. Body: %s", status, body)
	}

	var resp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("GraphQL: failed to parse response: %v", err)
	}
	if len(resp.Errors) > 0 {
		t.Errorf("GraphQL: unexpected errors: %v", resp.Errors)
	}
}
