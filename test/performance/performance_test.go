//go:build performance

package performance_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/server"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// Environment variable names for performance test configuration.
const (
	EnvServerURL = "INTEGRATION_SERVER_URL"
	EnvAPIKey    = "INTEGRATION_API_KEY"
	EnvBasicUser = "INTEGRATION_BASIC_USER"
	EnvBasicPass = "INTEGRATION_BASIC_PASS"
	EnvUseLocal  = "PERF_USE_LOCAL_SERVER"
)

// Default configuration values.
const (
	DefaultTimeout = 10 * time.Second
)

// testServerInfo holds the base URL and cleanup function for the
// server used during benchmarks.
type testServerInfo struct {
	baseURL string
	cleanup func()
}

// serverOnce ensures the test server is started only once.
var (
	serverOnce sync.Once
	serverInfo testServerInfo
)

// getOrStartServer returns the base URL of the server to benchmark.
// If INTEGRATION_SERVER_URL is set, it uses that. Otherwise, it
// starts a local in-process server.
func getOrStartServer(b *testing.B) string {
	b.Helper()

	if url := os.Getenv(EnvServerURL); url != "" {
		return url
	}

	serverOnce.Do(func() {
		serverInfo = startLocalServer(b)
	})

	return serverInfo.baseURL
}

// startLocalServer starts an in-process HTTP server for benchmarking
// and returns its base URL and a cleanup function.
func startLocalServer(b *testing.B) testServerInfo {
	b.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("Failed to find available port: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := &config.Config{
		ServerPort:      port,
		LogLevel:        "error",
		ShutdownTimeout: 5 * time.Second,
		MetricsEnabled:  true,
		AuthMode:        "none",
		TLSClientAuth:   "none",
	}

	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	srv := server.New(cfg, logger, itemStore, nil)

	go func() {
		if srvErr := srv.Start(); srvErr != nil &&
			srvErr != http.ErrServerClosed {
			b.Logf("Server error: %v", srvErr)
		}
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for server to be ready.
	waitCtx, waitCancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer waitCancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			b.Fatalf("Server did not become ready within timeout")
		case <-ticker.C:
			resp, reqErr := http.Get(baseURL + "/health")
			if reqErr == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					goto ready
				}
			}
		}
	}

ready:
	cleanup := func() {
		shutCtx, shutCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
	}

	return testServerInfo{
		baseURL: baseURL,
		cleanup: cleanup,
	}
}

// buildBenchAuthHeaders returns authentication headers for benchmarks.
func buildBenchAuthHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if apiKey := os.Getenv(EnvAPIKey); apiKey != "" {
		headers["X-API-Key"] = apiKey
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

// apiResponse is a generic API response envelope.
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

// BenchmarkHealthEndpoint measures the baseline latency of the
// health check endpoint.
func BenchmarkHealthEndpoint(b *testing.B) {
	baseURL := getOrStartServer(b)
	client := &http.Client{Timeout: DefaultTimeout}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(baseURL + "/health")
			if err != nil {
				b.Fatalf("Health request failed: %v", err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				b.Fatalf(
					"Health: expected 200, got %d",
					resp.StatusCode,
				)
			}
		}
	})
}

// BenchmarkAPIKeyAuth measures the overhead of API key
// authentication on a simple GET request.
func BenchmarkAPIKeyAuth(b *testing.B) {
	apiKey := os.Getenv(EnvAPIKey)
	if apiKey == "" {
		b.Skip("INTEGRATION_API_KEY not set, skipping")
	}

	baseURL := getOrStartServer(b)
	client := &http.Client{Timeout: DefaultTimeout}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest(
				http.MethodGet,
				baseURL+"/api/v1/items",
				nil,
			)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := client.Do(req)
			if err != nil {
				b.Fatalf("API key request failed: %v", err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				b.Fatalf(
					"API key auth: expected 200, got %d",
					resp.StatusCode,
				)
			}
		}
	})
}

// BenchmarkBasicAuth measures the overhead of HTTP Basic
// authentication (bcrypt) on a simple GET request.
func BenchmarkBasicAuth(b *testing.B) {
	user := os.Getenv(EnvBasicUser)
	pass := os.Getenv(EnvBasicPass)
	if user == "" || pass == "" {
		b.Skip("INTEGRATION_BASIC_USER/PASS not set, skipping")
	}

	baseURL := getOrStartServer(b)
	client := &http.Client{Timeout: DefaultTimeout}

	b.ResetTimer()
	for b.Loop() {
		req, _ := http.NewRequest(
			http.MethodGet,
			baseURL+"/api/v1/items",
			nil,
		)
		req.SetBasicAuth(user, pass)

		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Basic auth request failed: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b.Fatalf(
				"Basic auth: expected 200, got %d",
				resp.StatusCode,
			)
		}
	}
}

// BenchmarkCRUDCreate measures the latency of creating an item.
func BenchmarkCRUDCreate(b *testing.B) {
	baseURL := getOrStartServer(b)
	client := &http.Client{Timeout: DefaultTimeout}
	headers := buildBenchAuthHeaders()

	var counter atomic.Int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx := counter.Add(1)
			payload, _ := json.Marshal(map[string]any{
				"name":  fmt.Sprintf("Bench Item %d", idx),
				"price": 10.0,
			})

			req, _ := http.NewRequest(
				http.MethodPost,
				baseURL+"/api/v1/items",
				bytes.NewReader(payload),
			)
			for k, v := range headers {
				req.Header.Set(k, v)
			}

			resp, err := client.Do(req)
			if err != nil {
				b.Fatalf("Create request failed: %v", err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				b.Fatalf(
					"Create: expected 201, got %d",
					resp.StatusCode,
				)
			}
		}
	})
}

// BenchmarkCRUDRead measures the latency of reading an item.
func BenchmarkCRUDRead(b *testing.B) {
	baseURL := getOrStartServer(b)
	client := &http.Client{Timeout: DefaultTimeout}
	headers := buildBenchAuthHeaders()

	// Create an item to read.
	payload, _ := json.Marshal(map[string]any{
		"name":  "Bench Read Item",
		"price": 10.0,
	})

	req, _ := http.NewRequest(
		http.MethodPost,
		baseURL+"/api/v1/items",
		bytes.NewReader(payload),
	)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		b.Fatalf("Setup create failed: %v", err)
	}

	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		b.Fatalf("Failed to parse create response: %v", err)
	}

	var created itemResponse
	if err := json.Unmarshal(apiResp.Data, &created); err != nil {
		b.Fatalf("Failed to parse created item: %v", err)
	}

	itemURL := fmt.Sprintf(
		"%s/api/v1/items/%s", baseURL, created.ID,
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			readReq, _ := http.NewRequest(
				http.MethodGet, itemURL, nil,
			)
			for k, v := range headers {
				readReq.Header.Set(k, v)
			}

			readResp, readErr := client.Do(readReq)
			if readErr != nil {
				b.Fatalf("Read request failed: %v", readErr)
			}
			io.Copy(io.Discard, readResp.Body)
			readResp.Body.Close()

			if readResp.StatusCode != http.StatusOK {
				b.Fatalf(
					"Read: expected 200, got %d",
					readResp.StatusCode,
				)
			}
		}
	})
}

// BenchmarkConcurrentRequests measures throughput under concurrent
// load by running multiple goroutines making requests simultaneously.
func BenchmarkConcurrentRequests(b *testing.B) {
	baseURL := getOrStartServer(b)
	client := &http.Client{Timeout: DefaultTimeout}
	headers := buildBenchAuthHeaders()

	concurrencyLevels := []int{1, 5, 10, 25}

	for _, concurrency := range concurrencyLevels {
		b.Run(
			fmt.Sprintf("concurrency_%d", concurrency),
			func(b *testing.B) {
				b.SetParallelism(concurrency)
				b.ResetTimer()

				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						req, _ := http.NewRequest(
							http.MethodGet,
							baseURL+"/health",
							nil,
						)
						for k, v := range headers {
							req.Header.Set(k, v)
						}

						resp, err := client.Do(req)
						if err != nil {
							b.Fatalf(
								"Concurrent request failed: %v",
								err,
							)
						}
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
				})
			},
		)
	}
}
