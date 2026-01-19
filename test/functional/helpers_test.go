//go:build functional

// Package functional provides functional tests for the REST API and WebSocket server.
package functional

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/server"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// Environment variable names for test configuration.
const (
	EnvTestServerHost    = "TEST_SERVER_HOST"
	EnvTestServerPort    = "TEST_SERVER_PORT"
	EnvTestTimeout       = "TEST_TIMEOUT"
	EnvTestLogLevel      = "TEST_LOG_LEVEL"
	EnvTestMetricsEnable = "TEST_METRICS_ENABLED"
)

// Default test configuration values.
const (
	DefaultTestHost         = "localhost"
	DefaultTestPort         = 0 // 0 means auto-assign
	DefaultTestTimeout      = 30 * time.Second
	DefaultRequestTimeout   = 5 * time.Second
	DefaultWebSocketTimeout = 10 * time.Second
	DefaultShutdownTimeout  = 5 * time.Second
	DefaultLogLevel         = "error"
	DefaultMetricsEnabled   = false
)

// TestConfig holds test configuration loaded from environment.
type TestConfig struct {
	Host           string
	Port           int
	Timeout        time.Duration
	LogLevel       string
	MetricsEnabled bool
}

// LoadTestConfig loads test configuration from environment variables.
func LoadTestConfig() *TestConfig {
	cfg := &TestConfig{
		Host:           DefaultTestHost,
		Port:           DefaultTestPort,
		Timeout:        DefaultTestTimeout,
		LogLevel:       DefaultLogLevel,
		MetricsEnabled: DefaultMetricsEnabled,
	}

	if host := os.Getenv(EnvTestServerHost); host != "" {
		cfg.Host = host
	}

	if portStr := os.Getenv(EnvTestServerPort); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.Port = port
		}
	}

	if timeoutStr := os.Getenv(EnvTestTimeout); timeoutStr != "" {
		if timeout, err := time.ParseDuration(timeoutStr); err == nil {
			cfg.Timeout = timeout
		}
	}

	if logLevel := os.Getenv(EnvTestLogLevel); logLevel != "" {
		cfg.LogLevel = logLevel
	}

	if metricsStr := os.Getenv(EnvTestMetricsEnable); metricsStr != "" {
		if enabled, err := strconv.ParseBool(metricsStr); err == nil {
			cfg.MetricsEnabled = enabled
		}
	}

	return cfg
}

// TestServer wraps the server for testing purposes.
type TestServer struct {
	Server   *server.Server
	Store    *store.MemoryStore
	BaseURL  string
	WSURL    string
	Port     int
	listener net.Listener
	t        *testing.T
	mu       sync.Mutex
	started  bool
}

// NewTestServer creates a new test server instance.
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	testCfg := LoadTestConfig()

	// Find an available port
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", testCfg.Host, testCfg.Port))
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	// Create server configuration
	cfg := &config.Config{
		ServerPort:      port,
		LogLevel:        testCfg.LogLevel,
		ShutdownTimeout: DefaultShutdownTimeout,
		MetricsEnabled:  testCfg.MetricsEnabled,
	}

	// Create logger (use nop logger for tests to reduce noise)
	logger := zap.NewNop()

	// Create memory store
	itemStore := store.NewMemoryStore()

	// Create server
	srv := server.New(cfg, logger, itemStore)

	ts := &TestServer{
		Server:   srv,
		Store:    itemStore,
		BaseURL:  fmt.Sprintf("http://%s:%d", testCfg.Host, port),
		WSURL:    fmt.Sprintf("ws://%s:%d", testCfg.Host, port),
		Port:     port,
		listener: listener,
		t:        t,
	}

	return ts
}

// Start starts the test server.
func (ts *TestServer) Start() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.started {
		return
	}

	// Close the listener we used to find the port
	ts.listener.Close()

	// Start server in goroutine
	go func() {
		if err := ts.Server.Start(); err != nil && err != http.ErrServerClosed {
			ts.t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to be ready
	ts.waitForReady()
	ts.started = true
}

// waitForReady waits for the server to be ready to accept connections.
func (ts *TestServer) waitForReady() {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTestTimeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ts.t.Fatalf("Server did not become ready within timeout")
		case <-ticker.C:
			resp, err := http.Get(ts.BaseURL + "/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return
				}
			}
		}
	}
}

// Stop stops the test server.
func (ts *TestServer) Stop() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if !ts.started {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancel()

	if err := ts.Server.Shutdown(ctx); err != nil {
		ts.t.Logf("Server shutdown error: %v", err)
	}

	ts.started = false
}

// Reset clears the store for a fresh test.
func (ts *TestServer) Reset() {
	// Create a new store and update the server
	// Note: This is a simplified reset - in production you might need more sophisticated cleanup
	ts.Store = store.NewMemoryStore()
}

// HTTPClient provides a configured HTTP client for tests.
type HTTPClient struct {
	client  *http.Client
	baseURL string
	t       *testing.T
}

// NewHTTPClient creates a new HTTP client for testing.
func NewHTTPClient(t *testing.T, baseURL string) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: DefaultRequestTimeout,
		},
		baseURL: baseURL,
		t:       t,
	}
}

// Request represents an HTTP request configuration.
type Request struct {
	Method  string
	Path    string
	Body    interface{}
	Headers map[string]string
}

// Response represents an HTTP response.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Do executes an HTTP request and returns the response.
func (c *HTTPClient) Do(ctx context.Context, req Request) (*Response, error) {
	var bodyReader io.Reader
	if req.Body != nil {
		switch v := req.Body.(type) {
		case string:
			bodyReader = bytes.NewBufferString(v)
		case []byte:
			bodyReader = bytes.NewBuffer(v)
		default:
			jsonBody, err := json.Marshal(req.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewBuffer(jsonBody)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, c.baseURL+req.Path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default content type for requests with body
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Set custom headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}, nil
}

// Get performs a GET request.
func (c *HTTPClient) Get(ctx context.Context, path string, headers map[string]string) (*Response, error) {
	return c.Do(ctx, Request{
		Method:  http.MethodGet,
		Path:    path,
		Headers: headers,
	})
}

// Post performs a POST request.
func (c *HTTPClient) Post(ctx context.Context, path string, body interface{}, headers map[string]string) (*Response, error) {
	return c.Do(ctx, Request{
		Method:  http.MethodPost,
		Path:    path,
		Body:    body,
		Headers: headers,
	})
}

// Put performs a PUT request.
func (c *HTTPClient) Put(ctx context.Context, path string, body interface{}, headers map[string]string) (*Response, error) {
	return c.Do(ctx, Request{
		Method:  http.MethodPut,
		Path:    path,
		Body:    body,
		Headers: headers,
	})
}

// Delete performs a DELETE request.
func (c *HTTPClient) Delete(ctx context.Context, path string, headers map[string]string) (*Response, error) {
	return c.Do(ctx, Request{
		Method:  http.MethodDelete,
		Path:    path,
		Headers: headers,
	})
}

// APIResponse represents a generic API response structure.
type APIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ErrorResponse represents an error response structure.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// ItemResponse represents an item in API responses.
type ItemResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Price       float64   `json:"price"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// HealthResponse represents a health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// ParseAPIResponse parses an API response from bytes.
func ParseAPIResponse(body []byte) (*APIResponse, error) {
	var resp APIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}
	return &resp, nil
}

// ParseErrorResponse parses an error response from bytes.
func ParseErrorResponse(body []byte) (*ErrorResponse, error) {
	var resp ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse error response: %w", err)
	}
	return &resp, nil
}

// ParseItem parses an item from API response data.
func ParseItem(data json.RawMessage) (*ItemResponse, error) {
	var item ItemResponse
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("failed to parse item: %w", err)
	}
	return &item, nil
}

// ParseItems parses a list of items from API response data.
func ParseItems(data json.RawMessage) ([]ItemResponse, error) {
	// Handle empty or nil data (empty list case)
	if len(data) == 0 || string(data) == "null" {
		return []ItemResponse{}, nil
	}

	var items []ItemResponse
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to parse items: %w", err)
	}
	return items, nil
}

// ParseHealthResponse parses a health response from API response data.
func ParseHealthResponse(data json.RawMessage) (*HealthResponse, error) {
	var health HealthResponse
	if err := json.Unmarshal(data, &health); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %w", err)
	}
	return &health, nil
}

// CreateItemRequest represents a request to create an item.
type CreateItemRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Price       float64 `json:"price"`
}

// UpdateItemRequest represents a request to update an item.
type UpdateItemRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Price       float64 `json:"price"`
}

// AssertStatusCode asserts that the response has the expected status code.
func AssertStatusCode(t *testing.T, resp *Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Errorf("Expected status code %d, got %d. Body: %s", expected, resp.StatusCode, string(resp.Body))
	}
}

// AssertHeader asserts that the response has the expected header value.
func AssertHeader(t *testing.T, resp *Response, key, expected string) {
	t.Helper()
	actual := resp.Headers.Get(key)
	if actual != expected {
		t.Errorf("Expected header %s to be %q, got %q", key, expected, actual)
	}
}

// AssertSuccess asserts that the API response indicates success.
func AssertSuccess(t *testing.T, apiResp *APIResponse) {
	t.Helper()
	if !apiResp.Success {
		t.Errorf("Expected success=true, got false. Error: %s", apiResp.Error)
	}
}

// AssertError asserts that the API response indicates an error.
func AssertError(t *testing.T, apiResp *APIResponse) {
	t.Helper()
	if apiResp.Success {
		t.Error("Expected success=false, got true")
	}
}

// LogTestStart logs the start of a test.
func LogTestStart(t *testing.T, testID, testName string) {
	t.Helper()
	t.Logf("Starting test %s: %s", testID, testName)
}

// LogTestEnd logs the end of a test.
func LogTestEnd(t *testing.T, testID string) {
	t.Helper()
	t.Logf("Completed test %s", testID)
}
