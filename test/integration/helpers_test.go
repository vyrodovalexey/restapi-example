//go:build integration

// Package integration_test contains integration tests that exercise the
// server against the live docker-compose environment (Vault PKI + Keycloak).
//
// Unlike functional tests, these tests start in-process server instances
// configured per authentication mode and drive real credentials obtained
// from the running Keycloak (OIDC password grant) and Vault-issued mTLS
// certificates. This keeps the suite self-contained: no externally managed
// server process is required, only the docker-compose dependencies.
//
// All external-dependency lookups read INTEGRATION_* environment variables
// with sane defaults that match test/docker-compose/.env.test. When a
// required dependency is genuinely unreachable the affected test skips
// cleanly (t.Skip) rather than hard-failing.
package integration_test

import (
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

// Environment variable names for integration test configuration. These map
// directly to the values declared in test/docker-compose/.env.test.
const (
	EnvKeycloakURL = "INTEGRATION_KEYCLOAK_URL"
	EnvVaultAddr   = "VAULT_ADDR"

	EnvAPIKey    = "INTEGRATION_API_KEY"
	EnvBasicUser = "INTEGRATION_BASIC_USER"
	EnvBasicPass = "INTEGRATION_BASIC_PASS"

	EnvOIDCClientID     = "INTEGRATION_OIDC_CLIENT_ID"
	EnvOIDCClientSecret = "INTEGRATION_OIDC_CLIENT_SECRET" //nolint:gosec // env var name, not a credential
	EnvOIDCRealm        = "INTEGRATION_OIDC_REALM"
	EnvOIDCUsername     = "INTEGRATION_OIDC_USERNAME"
	EnvOIDCPassword     = "INTEGRATION_OIDC_PASSWORD" //nolint:gosec // env var name, not a credential
	EnvOIDCAdminUser    = "INTEGRATION_OIDC_ADMIN_USERNAME"
	EnvOIDCAdminPass    = "INTEGRATION_OIDC_ADMIN_PASSWORD" //nolint:gosec // env var name, not a credential

	EnvCertsDir   = "CERTS_DIR"
	EnvCACertPath = "INTEGRATION_CA_CERT_PATH"
	EnvClientCert = "INTEGRATION_CLIENT_CERT_PATH"
	EnvClientKey  = "INTEGRATION_CLIENT_KEY_PATH"
)

// Default configuration values matching .env.test. Tests favour these
// defaults so that running with the docker-compose env "just works".
const (
	DefaultKeycloakURL = "http://localhost:8090"
	DefaultVaultAddr   = "http://localhost:8200"

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

	// requestTimeout bounds individual HTTP requests in the suite.
	requestTimeout = 10 * time.Second
	// readyTimeout bounds the in-process server readiness wait.
	readyTimeout = 15 * time.Second
)

// getEnvOrDefault returns the value of the environment variable identified by
// key, or defaultVal if the variable is not set.
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// keycloakURL returns the Keycloak base URL (host view).
func keycloakURL() string {
	return getEnvOrDefault(EnvKeycloakURL, DefaultKeycloakURL)
}

// oidcRealm returns the configured Keycloak realm.
func oidcRealm() string {
	return getEnvOrDefault(EnvOIDCRealm, DefaultOIDCRealm)
}

// oidcIssuerURL returns the OIDC issuer URL as seen from the host.
//
// IMPORTANT: Keycloak's discovery document reports the issuer as the
// host-facing URL (http://localhost:8090/realms/<realm>). When the server
// runs in-process on the host, the OIDC verifier MUST use this host issuer so
// that the issuer check in the token verifier matches the `iss` claim in
// Keycloak-issued tokens. The container-internal issuer
// (http://keycloak_web:8090/...) used by .env.test only applies when the
// server runs inside the compose network.
func oidcIssuerURL() string {
	return strings.TrimRight(keycloakURL(), "/") + "/realms/" + oidcRealm()
}

// certsDir returns the directory containing Vault-issued mTLS certificates.
//
// CERTS_DIR in .env.test is "./certs", which is relative to the
// docker-compose directory (the container view). When running host-side
// tests from test/integration/, that relative path does not resolve. We
// therefore prefer CERTS_DIR only when it actually exists on disk; otherwise
// we fall back to the repo-relative default (../docker-compose/certs).
func certsDir() string {
	if dir := os.Getenv(EnvCertsDir); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
		// Common case: CERTS_DIR=./certs from .env.test → docker-compose view.
		if _, err := os.Stat("../docker-compose/" + strings.TrimPrefix(dir, "./")); err == nil {
			return "../docker-compose/" + strings.TrimPrefix(dir, "./")
		}
	}
	return DefaultCertsDir
}

// caCertPath returns the CA certificate path, honouring an explicit override.
func caCertPath() string {
	return getEnvOrDefault(EnvCACertPath, certsDir()+"/ca-cert.pem")
}

// serverCertPath returns the server certificate path.
func serverCertPath() string {
	return certsDir() + "/server-cert.pem"
}

// serverKeyPath returns the server key path.
func serverKeyPath() string {
	return certsDir() + "/server-key.pem"
}

// clientCertPath returns the (test) client certificate path.
func clientCertPath() string {
	return getEnvOrDefault(EnvClientCert, certsDir()+"/test-client-cert.pem")
}

// clientKeyPath returns the (test) client key path.
func clientKeyPath() string {
	return getEnvOrDefault(EnvClientKey, certsDir()+"/test-client-key.pem")
}

// apiKeyValue / apiKeyConfig return the API key and its server config string.
func apiKeyValue() string  { return getEnvOrDefault(EnvAPIKey, DefaultAPIKey) }
func apiKeyConfig() string { return apiKeyValue() + ":" + DefaultAPIKeyName }

// basicUser / basicPass return basic auth credentials.
func basicUser() string { return getEnvOrDefault(EnvBasicUser, DefaultBasicUser) }
func basicPass() string { return getEnvOrDefault(EnvBasicPass, DefaultBasicPass) }

// basicAuthConfig builds an in-test bcrypt-hashed user config from the
// configured credentials so the in-process server accepts them.
func basicAuthConfig(t *testing.T) string {
	t.Helper()
	return basicUser() + ":" + bcryptHash(t, basicPass())
}

// bcryptHash returns a bcrypt hash of the given password at minimum cost so
// the in-process basic authenticator accepts the configured credentials.
func bcryptHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("Failed to generate bcrypt hash: %v", err)
	}
	return string(hash)
}

// oidcClientID / oidcClientSecret / oidcUsername / oidcPassword return the
// OIDC password-grant parameters.
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

// testServer represents an in-process server started for an integration test.
type testServer struct {
	baseURL string
	srv     *server.Server
	cleanup func()
}

// startServer launches an in-process server bound to a random local port and
// waits (with backoff) for it to become ready. The returned cleanup function
// shuts the server down and must be deferred by the caller. It registers
// itself with t.Cleanup as a safety net.
func startServer(
	t *testing.T,
	cfg *config.Config,
	authenticator auth.Authenticator,
) *testServer {
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

	// Populate build_info so it appears in /metrics output, mirroring the
	// production wiring done in cmd/server/main.go via ldflags.
	observability.SetBuildInfo("integration-test", "test-commit", "test-time")

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
	base := fmt.Sprintf("%s://127.0.0.1:%d", scheme, port)

	ts := &testServer{
		baseURL: base,
		srv:     srv,
		cleanup: func() {
			ctx, cancel := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer cancel()
			_ = srv.Shutdown(ctx)
		},
	}

	ts.waitReady(t, cfg)
	t.Cleanup(ts.cleanup)

	return ts
}

// waitReady polls the server's readiness with exponential-ish backoff until
// it responds successfully or the readiness deadline elapses.
func (ts *testServer) waitReady(t *testing.T, cfg *config.Config) {
	t.Helper()

	var client *http.Client
	if cfg.TLSEnabled {
		c, err := newTLSClient(caCertPath(), clientCertPath(), clientKeyPath())
		if err != nil {
			t.Fatalf("Failed to build TLS client for readiness: %v", err)
		}
		client = c
	} else {
		client = &http.Client{Timeout: 2 * time.Second}
	}

	deadline := time.Now().Add(readyTimeout)
	delay := 25 * time.Millisecond

	for time.Now().Before(deadline) {
		// Public endpoints are always reachable regardless of auth mode.
		resp, err := client.Get(ts.baseURL + "/health")
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
		ts.baseURL, readyTimeout)
}

// newHTTPClient returns a plain HTTP client with a sensible timeout.
func newHTTPClient() *http.Client {
	return &http.Client{Timeout: requestTimeout}
}

// newTLSClient builds an *http.Client configured for mTLS using the provided
// CA, client certificate, and client key file paths.
func newTLSClient(
	caCert, clientCert, clientKey string,
) (*http.Client, error) {
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

// skipIfKeycloakUnavailable skips the test when the Keycloak token endpoint is
// not reachable.
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
		t.Skipf("Keycloak discovery returned %d at %s",
			resp.StatusCode, url)
	}
}

// keycloakReachable reports whether the Keycloak discovery endpoint responds.
func keycloakReachable() bool {
	url := keycloakURL() + "/realms/" + oidcRealm() +
		"/.well-known/openid-configuration"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// newNoCertTLSClient builds an HTTPS client that trusts the server CA but
// presents NO client certificate. It is used to assert that a require-client-
// auth server rejects the handshake.
func newNoCertTLSClient(t *testing.T) *http.Client {
	t.Helper()

	caPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		t.Fatalf("reading CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		t.Fatalf("failed to append CA cert to pool")
	}

	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caPool,
				ServerName: "restapi-server",
				MinVersion: tls.VersionTLS12,
			},
		},
	}
}

// skipIfCertsUnavailable skips the test when the Vault-issued mTLS certs are
// not present on disk.
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

// getKeycloakPasswordToken obtains an access token from Keycloak using the
// resource-owner password credentials grant. It retries with backoff to
// tolerate brief Keycloak unavailability (R3).
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
			tokenURL,
			"application/x-www-form-urlencoded",
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

		var tokenResp keycloakTokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return "", fmt.Errorf("decoding token response: %w", err)
		}
		if tokenResp.AccessToken == "" {
			return "", fmt.Errorf("empty access token in response")
		}
		return tokenResp.AccessToken, nil
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

// healthResponse represents the health endpoint response.
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// readyResponse represents the ready endpoint response.
type readyResponse struct {
	Status string `json:"status"`
}

// doRequest performs an HTTP request and returns the status code and body.
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
