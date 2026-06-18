//go:build e2e

// Package e2e_test contains end-to-end tests that drive complete user
// journeys against an in-process server wired to the live docker-compose
// environment (Vault PKI + Keycloak).
//
// Each test starts a real server instance configured for the relevant auth
// mode, then interacts with it exclusively through its public HTTP(S) API —
// the same way an external client would. Credentials come from the running
// Keycloak (OIDC password grant) and Vault-issued mTLS certificates.
//
// Configuration is read from INTEGRATION_* environment variables with
// defaults matching test/docker-compose/.env.test. Tests skip cleanly when a
// genuinely required dependency (Keycloak, certs) is unavailable.
package e2e_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/observability"
	"github.com/vyrodovalexey/restapi-example/internal/server"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// Environment variable names for E2E test configuration.
const (
	EnvKeycloakURL = "INTEGRATION_KEYCLOAK_URL"

	EnvAPIKey    = "INTEGRATION_API_KEY"
	EnvBasicUser = "INTEGRATION_BASIC_USER"
	EnvBasicPass = "INTEGRATION_BASIC_PASS"

	EnvOIDCClientID     = "INTEGRATION_OIDC_CLIENT_ID"
	EnvOIDCClientSecret = "INTEGRATION_OIDC_CLIENT_SECRET" //nolint:gosec // env var name
	EnvOIDCRealm        = "INTEGRATION_OIDC_REALM"
	EnvOIDCUsername     = "INTEGRATION_OIDC_USERNAME"
	EnvOIDCPassword     = "INTEGRATION_OIDC_PASSWORD" //nolint:gosec // env var name

	EnvCertsDir   = "CERTS_DIR"
	EnvCACertPath = "INTEGRATION_CA_CERT_PATH"
	EnvClientCert = "INTEGRATION_CLIENT_CERT_PATH"
	EnvClientKey  = "INTEGRATION_CLIENT_KEY_PATH"
)

// Default configuration values matching .env.test.
const (
	DefaultKeycloakURL = "http://localhost:8090"

	DefaultAPIKey      = "test-key"
	DefaultAPIKeyName  = "test-service"
	DefaultBasicUser   = "testuser"
	DefaultBasicPass   = "password"
	DefaultOIDCClient  = "restapi-server"
	DefaultOIDCSecret  = "restapi-server-secret"
	DefaultOIDCRealm   = "restapi-test"
	DefaultOIDCUser    = "test-user"
	DefaultOIDCUserPwd = "test-password"

	DefaultCertsDir = "../docker-compose/certs"

	requestTimeout = 15 * time.Second
	readyTimeout   = 15 * time.Second
)

// getEnvOrDefault returns the environment value for key, or defaultVal.
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func keycloakURL() string { return getEnvOrDefault(EnvKeycloakURL, DefaultKeycloakURL) }
func oidcRealm() string   { return getEnvOrDefault(EnvOIDCRealm, DefaultOIDCRealm) }

// oidcIssuerURL returns the host-facing OIDC issuer URL. The OIDC verifier
// MUST use this (not the container-internal keycloak_web host) so the issuer
// check matches the `iss` claim in Keycloak-issued tokens for host-run tests.
func oidcIssuerURL() string {
	return strings.TrimRight(keycloakURL(), "/") + "/realms/" + oidcRealm()
}

// certsDir resolves the Vault-issued cert directory robustly. CERTS_DIR in
// .env.test ("./certs") is the docker-compose (container) view, so prefer it
// only when it exists, otherwise fall back to the repo-relative default.
func certsDir() string {
	if dir := os.Getenv(EnvCertsDir); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
		alt := "../docker-compose/" + strings.TrimPrefix(dir, "./")
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return DefaultCertsDir
}

func caCertPath() string {
	return getEnvOrDefault(EnvCACertPath, certsDir()+"/ca-cert.pem")
}
func serverCertPath() string { return certsDir() + "/server-cert.pem" }
func serverKeyPath() string  { return certsDir() + "/server-key.pem" }
func clientCertPath() string {
	return getEnvOrDefault(EnvClientCert, certsDir()+"/test-client-cert.pem")
}
func clientKeyPath() string {
	return getEnvOrDefault(EnvClientKey, certsDir()+"/test-client-key.pem")
}

func apiKeyValue() string  { return getEnvOrDefault(EnvAPIKey, DefaultAPIKey) }
func apiKeyConfig() string { return apiKeyValue() + ":" + DefaultAPIKeyName }
func basicUser() string    { return getEnvOrDefault(EnvBasicUser, DefaultBasicUser) }
func basicPass() string    { return getEnvOrDefault(EnvBasicPass, DefaultBasicPass) }

func oidcClientID() string {
	return getEnvOrDefault(EnvOIDCClientID, DefaultOIDCClient)
}
func oidcClientSecret() string {
	return getEnvOrDefault(EnvOIDCClientSecret, DefaultOIDCSecret)
}
func oidcUsername() string {
	return getEnvOrDefault(EnvOIDCUsername, DefaultOIDCUser)
}
func oidcPassword() string {
	return getEnvOrDefault(EnvOIDCPassword, DefaultOIDCUserPwd)
}

// bcryptHash returns a bcrypt hash for the given password at minimum cost.
func bcryptHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("Failed to generate bcrypt hash: %v", err)
	}
	return string(hash)
}

func basicAuthConfig(t *testing.T) string {
	t.Helper()
	return basicUser() + ":" + bcryptHash(t, basicPass())
}

// e2eServer holds an in-process server started for an E2E journey.
type e2eServer struct {
	baseURL string
	srv     *server.Server
	cleanup func()
}

// startServer launches an in-process server on a random port and waits for it
// to become ready. It registers cleanup with t.Cleanup.
func startServer(
	t *testing.T,
	cfg *config.Config,
	authenticator auth.Authenticator,
) *e2eServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	cfg.ServerPort = port
	cfg.ProbePort = 0
	if cfg.LogLevel == "" {
		cfg.LogLevel = "error"
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 5 * time.Second
	}

	observability.SetBuildInfo("e2e-test", "test-commit", "test-time")

	itemStore := store.NewInstrumentedStore(store.NewMemoryStore())
	srv := server.New(cfg, zap.NewNop(), itemStore, authenticator)

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			t.Logf("server exited: %v", err)
		}
	}()

	scheme := "http"
	if cfg.TLSEnabled {
		scheme = "https"
	}
	es := &e2eServer{
		baseURL: fmt.Sprintf("%s://127.0.0.1:%d", scheme, port),
		srv:     srv,
		cleanup: func() {
			ctx, cancel := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer cancel()
			_ = srv.Shutdown(ctx)
		},
	}

	es.waitReady(t, cfg)
	t.Cleanup(es.cleanup)

	return es
}

// waitReady polls /health with backoff until the server responds.
func (es *e2eServer) waitReady(t *testing.T, cfg *config.Config) {
	t.Helper()

	var client *http.Client
	if cfg.TLSEnabled {
		c, err := newTLSClient(caCertPath(), clientCertPath(), clientKeyPath())
		if err != nil {
			t.Fatalf("Failed to build readiness TLS client: %v", err)
		}
		client = c
	} else {
		client = &http.Client{Timeout: 2 * time.Second}
	}

	deadline := time.Now().Add(readyTimeout)
	delay := 25 * time.Millisecond
	for time.Now().Before(deadline) {
		resp, err := client.Get(es.baseURL + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(delay)
		if delay < 400*time.Millisecond {
			delay *= 2
		}
	}
	t.Fatalf("server at %s did not become ready within %v",
		es.baseURL, readyTimeout)
}

// newHTTPClient returns a plain HTTP client with a sensible timeout.
func newHTTPClient() *http.Client {
	return &http.Client{Timeout: requestTimeout}
}

// newTLSClient builds an *http.Client configured for mTLS.
func newTLSClient(caCert, clientCert, clientKey string) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Errorf("loading client key pair: %w", err)
	}
	caPEM, err := os.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to append CA cert to pool")
	}
	return &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caPool,
				ServerName:   "restapi-server",
				MinVersion:   tls.VersionTLS12,
			},
		},
	}, nil
}

// skipIfKeycloakUnavailable skips when the Keycloak discovery endpoint is
// unreachable.
func skipIfKeycloakUnavailable(t *testing.T) {
	t.Helper()
	url := keycloakURL() + "/realms/" + oidcRealm() +
		"/.well-known/openid-configuration"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Skipf("Keycloak unavailable at %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Keycloak discovery returned %d", resp.StatusCode)
	}
}

// skipIfCertsUnavailable skips when the Vault-issued certs are not present.
func skipIfCertsUnavailable(t *testing.T) {
	t.Helper()
	for _, p := range []string{
		caCertPath(), serverCertPath(), serverKeyPath(),
		clientCertPath(), clientKeyPath(),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("mTLS cert %s unavailable: %v", p, err)
		}
	}
}

// keycloakTokenResponse models the Keycloak token endpoint response.
type keycloakTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// getKeycloakPasswordToken obtains a token via the password grant, retrying
// with backoff to tolerate brief Keycloak unavailability.
func getKeycloakPasswordToken(
	clientID, clientSecret, username, password string,
) (string, error) {
	tokenURL := fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/token",
		keycloakURL(), oidcRealm(),
	)
	form := fmt.Sprintf(
		"grant_type=password&client_id=%s&client_secret=%s"+
			"&username=%s&password=%s&scope=openid",
		clientID, clientSecret, username, password,
	)

	client := &http.Client{Timeout: requestTimeout}
	var lastErr error
	delay := 200 * time.Millisecond
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
		}
		resp, err := client.Post(
			tokenURL, "application/x-www-form-urlencoded",
			strings.NewReader(form),
		)
		if err != nil {
			lastErr = fmt.Errorf("requesting token: %w", err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf(
				"token request failed with status %d: %s",
				resp.StatusCode, string(body),
			)
			continue
		}
		var tr keycloakTokenResponse
		if err := json.Unmarshal(body, &tr); err != nil {
			return "", fmt.Errorf("decoding token response: %w", err)
		}
		if tr.AccessToken == "" {
			return "", fmt.Errorf("empty access token")
		}
		return tr.AccessToken, nil
	}
	return "", lastErr
}

// apiResponse is the generic API response envelope.
type apiResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// itemResponse represents an item returned by the API.
type itemResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Price       float64   `json:"price"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// createItemRequest is the payload for creating an item.
type createItemRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Price       float64 `json:"price"`
}

// updateItemRequest is the payload for updating an item.
type updateItemRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Price       float64 `json:"price"`
}

// doRequest performs an HTTP request and returns status code and body.
func doRequest(
	t *testing.T,
	client *http.Client,
	method, url string,
	body io.Reader,
	headers map[string]string,
) (int, []byte) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request %s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	return resp.StatusCode, respBody
}

// createItem creates an item via the API and returns the parsed response.
func createItem(
	t *testing.T,
	client *http.Client,
	base string,
	headers map[string]string,
	item createItemRequest,
) itemResponse {
	t.Helper()
	payload, _ := json.Marshal(item)
	status, body := doRequest(
		t, client, http.MethodPost, base+"/api/v1/items",
		bytes.NewReader(payload), headers,
	)
	if status != http.StatusCreated {
		t.Fatalf("createItem: expected 201, got %d. Body: %s", status, body)
	}
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("createItem: failed to parse response: %v", err)
	}
	var created itemResponse
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		t.Fatalf("createItem: failed to parse item: %v", err)
	}
	return created
}

// deleteItem deletes an item by ID (best-effort cleanup).
func deleteItem(
	t *testing.T,
	client *http.Client,
	base, id string,
	headers map[string]string,
) {
	t.Helper()
	url := fmt.Sprintf("%s/api/v1/items/%s", base, id)
	status, body := doRequest(t, client, http.MethodDelete, url, nil, headers)
	if status != http.StatusNoContent {
		t.Logf("deleteItem cleanup: expected 204, got %d. Body: %s", status, body)
	}
}

// fullCRUDJourney drives create → read → update → verify → delete → verify.
func fullCRUDJourney(
	t *testing.T,
	client *http.Client,
	base string,
	headers map[string]string,
) {
	t.Helper()

	created := createItem(t, client, base, headers, createItemRequest{
		Name:        "E2E Journey Item",
		Description: "created during e2e",
		Price:       99.99,
	})
	if created.ID == "" {
		t.Fatal("Created item has empty ID")
	}
	itemURL := fmt.Sprintf("%s/api/v1/items/%s", base, created.ID)

	status, body := doRequest(t, client, http.MethodGet, itemURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("Read: expected 200, got %d. Body: %s", status, body)
	}

	updatePayload, _ := json.Marshal(updateItemRequest{
		Name:        "E2E Journey Item Updated",
		Description: "updated during e2e",
		Price:       149.99,
	})
	status, _ = doRequest(
		t, client, http.MethodPut, itemURL,
		bytes.NewReader(updatePayload), headers,
	)
	if status != http.StatusOK {
		t.Fatalf("Update: expected 200, got %d", status)
	}

	status, body = doRequest(t, client, http.MethodGet, itemURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("Verify update: expected 200, got %d", status)
	}
	var verifyResp apiResponse
	_ = json.Unmarshal(body, &verifyResp)
	var verifyItem itemResponse
	_ = json.Unmarshal(verifyResp.Data, &verifyItem)
	if verifyItem.Name != "E2E Journey Item Updated" {
		t.Errorf("Verify: unexpected name %q", verifyItem.Name)
	}

	status, _ = doRequest(t, client, http.MethodDelete, itemURL, nil, headers)
	if status != http.StatusNoContent {
		t.Fatalf("Delete: expected 204, got %d", status)
	}

	status, _ = doRequest(t, client, http.MethodGet, itemURL, nil, headers)
	if status != http.StatusNotFound {
		t.Errorf("Verify delete: expected 404, got %d", status)
	}
}

// jsonHeaders merges Content-Type with the provided auth headers.
func jsonHeaders(auth map[string]string) map[string]string {
	h := map[string]string{"Content-Type": "application/json"}
	for k, v := range auth {
		h[k] = v
	}
	return h
}
