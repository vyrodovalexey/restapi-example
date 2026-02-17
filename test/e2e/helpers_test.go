//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// Environment variable names for E2E test configuration.
const (
	EnvServerURL = "INTEGRATION_SERVER_URL"
	EnvAPIKey    = "INTEGRATION_API_KEY"
	EnvBasicUser = "INTEGRATION_BASIC_USER"
	EnvBasicPass = "INTEGRATION_BASIC_PASS"
)

// Default configuration values.
const (
	DefaultServerURL = "http://localhost:8080"
	DefaultTimeout   = 15 * time.Second
)

// getEnvOrDefault returns the value of the environment variable
// identified by key, or defaultVal if the variable is not set.
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// e2eServerURL returns the base URL of the server under test.
func e2eServerURL() string {
	return getEnvOrDefault(EnvServerURL, DefaultServerURL)
}

// skipIfServerUnavailable checks whether the server is reachable
// and skips the test if it is not.
func skipIfServerUnavailable(t *testing.T) {
	t.Helper()

	base := e2eServerURL()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(base + "/health")
	if err != nil {
		t.Skipf("Server unavailable at %s: %v", base, err)
	}
	resp.Body.Close()
}

// newHTTPClient returns an *http.Client with a sensible timeout.
func newHTTPClient() *http.Client {
	return &http.Client{Timeout: DefaultTimeout}
}

// apiResponse is a generic API response envelope.
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

// buildAuthHeaders returns a header map populated with authentication
// credentials from environment variables, if available.
func buildAuthHeaders(t *testing.T) map[string]string {
	t.Helper()

	headers := map[string]string{
		"Content-Type": "application/json",
	}

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

// createItem is a helper that creates an item and returns its parsed
// response. It fails the test on error.
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
		t, client, http.MethodPost,
		base+"/api/v1/items",
		bytes.NewReader(payload), headers,
	)

	if status != http.StatusCreated {
		t.Fatalf(
			"createItem: expected 201, got %d. Body: %s",
			status, body,
		)
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

// deleteItem is a helper that deletes an item by ID.
func deleteItem(
	t *testing.T,
	client *http.Client,
	base, id string,
	headers map[string]string,
) {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/items/%s", base, id)
	status, body := doRequest(
		t, client, http.MethodDelete, url, nil, headers,
	)

	if status != http.StatusNoContent {
		t.Logf(
			"deleteItem cleanup: expected 204, got %d. Body: %s",
			status, body,
		)
	}
}
