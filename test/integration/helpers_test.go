//go:build integration

package integration_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Environment variable names for integration test configuration.
const (
	EnvServerURL   = "INTEGRATION_SERVER_URL"
	EnvCACertPath  = "INTEGRATION_CA_CERT_PATH"
	EnvClientCert  = "INTEGRATION_CLIENT_CERT_PATH"
	EnvClientKey   = "INTEGRATION_CLIENT_KEY_PATH"
	EnvKeycloakURL = "INTEGRATION_KEYCLOAK_URL"
	EnvAPIKey      = "INTEGRATION_API_KEY"
	EnvBasicUser   = "INTEGRATION_BASIC_USER"
	EnvBasicPass   = "INTEGRATION_BASIC_PASS"
)

// Default configuration values.
const (
	DefaultServerURL   = "http://localhost:8080"
	DefaultKeycloakURL = "http://localhost:8090"
	DefaultTimeout     = 10 * time.Second
)

// getEnvOrDefault returns the value of the environment variable
// identified by key, or defaultVal if the variable is not set.
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// skipIfServiceUnavailable checks whether the service at the given
// URL is reachable and skips the test if it is not.
func skipIfServiceUnavailable(t *testing.T, url string) {
	t.Helper()

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Skipf("Service unavailable at %s: %v", url, err)
	}
	resp.Body.Close()
}

// skipIfServiceUnavailableTLS checks whether the TLS service at the
// given URL is reachable using the provided client certificates and
// skips the test if it is not.
func skipIfServiceUnavailableTLS(
	t *testing.T,
	url, caCert, clientCert, clientKey string,
) {
	t.Helper()

	client, err := createTLSClient(caCert, clientCert, clientKey)
	if err != nil {
		t.Skipf("Cannot create TLS client: %v", err)
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Skipf("TLS service unavailable at %s: %v", url, err)
	}
	resp.Body.Close()
}

// createHTTPClient returns an *http.Client with a sensible timeout
// for integration tests.
func createHTTPClient() *http.Client {
	return &http.Client{Timeout: DefaultTimeout}
}

// createTLSClient builds an *http.Client configured for mTLS using
// the provided CA certificate, client certificate, and client key
// file paths.
func createTLSClient(
	caCert, clientCert, clientKey string,
) (*http.Client, error) {
	cert, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Errorf("loading client key pair: %w", err)
	}

	caCertPEM, err := os.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to append CA cert to pool")
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}

	return &http.Client{
		Timeout: DefaultTimeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}

// keycloakTokenResponse represents the token endpoint response from
// Keycloak.
type keycloakTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// getKeycloakToken obtains an access token from Keycloak using the
// client_credentials grant type.
func getKeycloakToken(
	keycloakURL, realm, clientID, clientSecret string,
) (string, error) {
	tokenURL := fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/token",
		keycloakURL, realm,
	)

	body := fmt.Sprintf(
		"grant_type=client_credentials&client_id=%s&client_secret=%s",
		clientID, clientSecret,
	)

	client := &http.Client{Timeout: DefaultTimeout}
	resp, err := client.Post(
		tokenURL,
		"application/x-www-form-urlencoded",
		strings.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("requesting token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf(
			"token request failed with status %d: %s",
			resp.StatusCode, string(respBody),
		)
	}

	var tokenResp keycloakTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// getKeycloakPasswordToken obtains an access token from Keycloak
// using the resource owner password credentials (password) grant type.
func getKeycloakPasswordToken(
	keycloakURL, realm, clientID, clientSecret, username, password string,
) (string, error) {
	tokenURL := fmt.Sprintf(
		"%s/realms/%s/protocol/openid-connect/token",
		keycloakURL, realm,
	)

	body := fmt.Sprintf(
		"grant_type=password&client_id=%s&client_secret=%s"+
			"&username=%s&password=%s",
		clientID, clientSecret, username, password,
	)

	client := &http.Client{Timeout: DefaultTimeout}
	resp, err := client.Post(
		tokenURL,
		"application/x-www-form-urlencoded",
		strings.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("requesting password token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf(
			"password token request failed with status %d: %s",
			resp.StatusCode, string(respBody),
		)
	}

	var tokenResp keycloakTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding password token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// apiResponse is a generic API response envelope used for parsing
// integration test responses.
type apiResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// errorResponse represents an error response from the API.
type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
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

// doRequest is a convenience wrapper that performs an HTTP request and
// returns the status code and body bytes.
func doRequest(
	t *testing.T,
	client *http.Client,
	method, url string,
	body io.Reader,
	headers map[string]string,
) (int, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
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
