// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

func TestNew(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore)

	// Assert
	if server == nil {
		t.Fatal("New() returned nil")
	}
	if server.router == nil {
		t.Error("router should not be nil")
	}
	if server.config == nil {
		t.Error("config should not be nil")
	}
	if server.logger == nil {
		t.Error("logger should not be nil")
	}
	if server.httpServer == nil {
		t.Error("httpServer should not be nil")
	}
	if server.wsHandler == nil {
		t.Error("wsHandler should not be nil")
	}
}

func TestNew_MetricsDisabled(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore)

	// Assert
	if server == nil {
		t.Fatal("New() returned nil")
	}

	// Metrics endpoint should not be available
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Metrics endpoint status = %d, want %d when metrics disabled", rr.Code, http.StatusNotFound)
	}
}

func TestNew_MetricsEnabled(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore)

	// Assert
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Metrics endpoint status = %d, want %d when metrics enabled", rr.Code, http.StatusOK)
	}
}

func TestServer_Router(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	// Act
	router := server.Router()

	// Assert
	if router == nil {
		t.Error("Router() returned nil")
	}
	if router != server.router {
		t.Error("Router() should return the server's router")
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	// Act
	server.router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Health endpoint status = %d, want %d", rr.Code, http.StatusOK)
	}

	var response model.APIResponse[map[string]string]
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.Success {
		t.Error("Health check should return success")
	}
}

func TestServer_RESTEndpoints(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "list items",
			method:     http.MethodGet,
			path:       "/api/v1/items",
			wantStatus: http.StatusOK,
		},
		{
			name:       "get item - not found",
			method:     http.MethodGet,
			path:       "/api/v1/items/non-existent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()

			// Act
			server.router.ServeHTTP(rr, req)

			// Assert
			if rr.Code != tt.wantStatus {
				t.Errorf("%s %s status = %d, want %d", tt.method, tt.path, rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestServer_WebSocketEndpoint(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	// Test that WebSocket endpoint is registered
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()

	// Act
	server.router.ServeHTTP(rr, req)

	// Assert - Should not be 404 (will fail upgrade but route exists)
	if rr.Code == http.StatusNotFound {
		t.Error("WebSocket endpoint /ws not found")
	}
}

func TestServer_Shutdown(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8090,
		LogLevel:        "info",
		ShutdownTimeout: 5 * time.Second,
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	// Start server in background
	go func() {
		_ = server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Act
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)

	// Assert
	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestServer_ShutdownWithTimeout(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8091,
		LogLevel:        "info",
		ShutdownTimeout: 5 * time.Second,
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	// Start server in background
	go func() {
		_ = server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Act - Shutdown with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// This might or might not error depending on timing
	_ = server.Shutdown(ctx)

	// Assert - No panic should occur
}

func TestServer_HTTPServerConfiguration(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore)

	// Assert
	if server.httpServer.Addr != ":8080" {
		t.Errorf("httpServer.Addr = %s, want :8080", server.httpServer.Addr)
	}
	if server.httpServer.ReadTimeout != 15*time.Second {
		t.Errorf("httpServer.ReadTimeout = %v, want 15s", server.httpServer.ReadTimeout)
	}
	if server.httpServer.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("httpServer.ReadHeaderTimeout = %v, want 5s", server.httpServer.ReadHeaderTimeout)
	}
	if server.httpServer.WriteTimeout != 15*time.Second {
		t.Errorf("httpServer.WriteTimeout = %v, want 15s", server.httpServer.WriteTimeout)
	}
	if server.httpServer.IdleTimeout != 60*time.Second {
		t.Errorf("httpServer.IdleTimeout = %v, want 60s", server.httpServer.IdleTimeout)
	}
	if server.httpServer.MaxHeaderBytes != 1<<20 {
		t.Errorf("httpServer.MaxHeaderBytes = %d, want %d", server.httpServer.MaxHeaderBytes, 1<<20)
	}
}

func TestServer_MiddlewareApplied(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	// Act
	server.router.ServeHTTP(rr, req)

	// Assert - Check that middleware is applied
	// Request ID should be set
	if rr.Header().Get("X-Request-ID") == "" {
		t.Error("X-Request-ID header should be set by middleware")
	}

	// CORS headers should be set
	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("CORS headers should be set by middleware")
	}
}

func TestServer_CORSPreflight(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/items", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := httptest.NewRecorder()

	// Act
	server.router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusNoContent && rr.Code != http.StatusOK && rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Preflight status = %d, want 204, 200, or 405", rr.Code)
	}
}

func TestServer_RecoveryMiddleware(t *testing.T) {
	// This test verifies that the recovery middleware is in place
	// by checking that the server doesn't crash on normal requests

	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	// Make multiple requests to ensure stability
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		// Act
		server.router.ServeHTTP(rr, req)

		// Assert
		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: status = %d, want %d", i, rr.Code, http.StatusOK)
		}
	}
}

func TestServer_ContentType(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	// Act
	server.router.ServeHTTP(rr, req)

	// Assert
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}
}

func TestServer_DifferentPorts(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{"default port", 8080, ":8080"},
		{"custom port", 3000, ":3000"},
		{"high port", 65535, ":65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{
				ServerPort:      tt.port,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				MetricsEnabled:  false,
			}
			logger := zap.NewNop()
			itemStore := store.NewMemoryStore()

			// Act
			server := New(cfg, logger, itemStore)

			// Assert
			if server.httpServer.Addr != tt.want {
				t.Errorf("httpServer.Addr = %s, want %s", server.httpServer.Addr, tt.want)
			}
		})
	}
}
