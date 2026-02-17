// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// testAuthenticator is a mock authenticator for server tests.
type testAuthenticator struct {
	info   *auth.AuthInfo
	err    error
	method auth.AuthMethod
}

func (a *testAuthenticator) Authenticate(_ *http.Request) (*auth.AuthInfo, error) {
	return a.info, a.err
}

func (a *testAuthenticator) Method() auth.AuthMethod {
	return a.method
}

// generateTestCert creates a self-signed certificate and key in the given directory.
// Returns the paths to the cert and key files.
func generateTestCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("failed to create cert file: %v", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("failed to encode cert: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}

	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("failed to create key file: %v", err)
	}
	defer keyFile.Close()

	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("failed to encode key: %v", err)
	}

	return certPath, keyPath
}

// generateTestCA creates a CA certificate in the given directory.
// Returns the path to the CA cert file.
func generateTestCA(t *testing.T, dir string) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}

	caPath := filepath.Join(dir, "ca.pem")

	caFile, err := os.Create(caPath)
	if err != nil {
		t.Fatalf("failed to create CA file: %v", err)
	}
	defer caFile.Close()

	if err := pem.Encode(caFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatalf("failed to encode CA cert: %v", err)
	}

	return caPath
}

func TestNew(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 5 * time.Second,
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 5 * time.Second,
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

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
				ProbePort:       0,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				MetricsEnabled:  false,
			}
			logger := zap.NewNop()
			itemStore := store.NewMemoryStore()

			// Act
			server := New(cfg, logger, itemStore, nil)

			// Assert
			if server.httpServer.Addr != tt.want {
				t.Errorf("httpServer.Addr = %s, want %s", server.httpServer.Addr, tt.want)
			}
		})
	}
}

func TestBuildTLSConfig_ValidCert(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      keyPath,
		TLSClientAuth:   "none",
	}
	logger := zap.NewNop()

	s := &Server{
		config: cfg,
		logger: logger,
	}

	// Act
	tlsConfig, err := s.buildTLSConfig()

	// Assert
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if tlsConfig == nil {
		t.Fatal("buildTLSConfig() returned nil")
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("Certificates count = %d, want 1", len(tlsConfig.Certificates))
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", tlsConfig.MinVersion, tls.VersionTLS12)
	}
	if tlsConfig.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %d, want %d", tlsConfig.ClientAuth, tls.NoClientCert)
	}
}

func TestBuildTLSConfig_InvalidCertPath(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     "/nonexistent/cert.pem",
		TLSKeyPath:      "/nonexistent/key.pem",
		TLSClientAuth:   "none",
	}
	logger := zap.NewNop()

	s := &Server{
		config: cfg,
		logger: logger,
	}

	// Act
	tlsConfig, err := s.buildTLSConfig()

	// Assert
	if err == nil {
		t.Fatal("buildTLSConfig() expected error for invalid cert path")
	}
	if tlsConfig != nil {
		t.Error("buildTLSConfig() should return nil on error")
	}
	if !strings.Contains(err.Error(), "loading TLS key pair") {
		t.Errorf("error = %v, want to contain 'loading TLS key pair'", err)
	}
}

func TestBuildTLSConfig_InvalidKeyPath(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	certPath, _ := generateTestCert(t, dir)

	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      "/nonexistent/key.pem",
		TLSClientAuth:   "none",
	}
	logger := zap.NewNop()

	s := &Server{
		config: cfg,
		logger: logger,
	}

	// Act
	tlsConfig, err := s.buildTLSConfig()

	// Assert
	if err == nil {
		t.Fatal("buildTLSConfig() expected error for invalid key path")
	}
	if tlsConfig != nil {
		t.Error("buildTLSConfig() should return nil on error")
	}
}

func TestBuildTLSConfig_ClientAuthModes(t *testing.T) {
	tests := []struct {
		name           string
		clientAuth     string
		wantClientAuth tls.ClientAuthType
	}{
		{"require", "require", tls.RequireAndVerifyClientCert},
		{"request", "request", tls.RequestClientCert},
		{"none", "none", tls.NoClientCert},
		{"default (empty)", "", tls.NoClientCert},
		{"unknown value", "unknown", tls.NoClientCert},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			certPath, keyPath := generateTestCert(t, dir)

			cfg := &config.Config{
				ServerPort:      8080,
				ProbePort:       0,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				TLSEnabled:      true,
				TLSCertPath:     certPath,
				TLSKeyPath:      keyPath,
				TLSClientAuth:   tt.clientAuth,
			}
			logger := zap.NewNop()

			s := &Server{
				config: cfg,
				logger: logger,
			}

			// Act
			tlsConfig, err := s.buildTLSConfig()

			// Assert
			if err != nil {
				t.Fatalf("buildTLSConfig() error = %v", err)
			}
			if tlsConfig.ClientAuth != tt.wantClientAuth {
				t.Errorf("ClientAuth = %d, want %d", tlsConfig.ClientAuth, tt.wantClientAuth)
			}
		})
	}
}

func TestBuildTLSConfig_WithCAPath(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)
	caPath := generateTestCA(t, dir)

	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      keyPath,
		TLSCAPath:       caPath,
		TLSClientAuth:   "require",
	}
	logger := zap.NewNop()

	s := &Server{
		config: cfg,
		logger: logger,
	}

	// Act
	tlsConfig, err := s.buildTLSConfig()

	// Assert
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if tlsConfig.ClientCAs == nil {
		t.Error("ClientCAs should be set when TLSCAPath is provided")
	}
}

func TestBuildTLSConfig_InvalidCAPath(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      keyPath,
		TLSCAPath:       "/nonexistent/ca.pem",
		TLSClientAuth:   "require",
	}
	logger := zap.NewNop()

	s := &Server{
		config: cfg,
		logger: logger,
	}

	// Act
	tlsConfig, err := s.buildTLSConfig()

	// Assert
	if err == nil {
		t.Fatal("buildTLSConfig() expected error for invalid CA path")
	}
	if tlsConfig != nil {
		t.Error("buildTLSConfig() should return nil on error")
	}
	if !strings.Contains(err.Error(), "reading TLS CA cert") {
		t.Errorf("error = %v, want to contain 'reading TLS CA cert'", err)
	}
}

func TestBuildTLSConfig_InvalidCACert(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	// Write invalid CA cert content
	invalidCAPath := filepath.Join(dir, "invalid-ca.pem")
	if err := os.WriteFile(invalidCAPath, []byte("not a valid certificate"), 0o600); err != nil {
		t.Fatalf("failed to write invalid CA file: %v", err)
	}

	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      keyPath,
		TLSCAPath:       invalidCAPath,
		TLSClientAuth:   "require",
	}
	logger := zap.NewNop()

	s := &Server{
		config: cfg,
		logger: logger,
	}

	// Act
	tlsConfig, err := s.buildTLSConfig()

	// Assert
	if err == nil {
		t.Fatal("buildTLSConfig() expected error for invalid CA cert content")
	}
	if tlsConfig != nil {
		t.Error("buildTLSConfig() should return nil on error")
	}
	if !strings.Contains(err.Error(), "parsing TLS CA cert") {
		t.Errorf("error = %v, want to contain 'parsing TLS CA cert'", err)
	}
}

func TestSetupHTTPServer_WithTLS(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      keyPath,
		TLSClientAuth:   "none",
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

	// Assert
	if server.httpServer.TLSConfig == nil {
		t.Error("TLSConfig should be set when TLS is enabled")
	}
	if server.initErr != nil {
		t.Errorf("initErr should be nil, got %v", server.initErr)
	}
}

func TestSetupHTTPServer_WithTLS_InvalidCert(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     "/nonexistent/cert.pem",
		TLSKeyPath:      "/nonexistent/key.pem",
		TLSClientAuth:   "none",
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

	// Assert - initErr should be set
	if server.initErr == nil {
		t.Error("initErr should be set when TLS cert is invalid")
	}
}

func TestServer_Start_WithInitErr(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     "/nonexistent/cert.pem",
		TLSKeyPath:      "/nonexistent/key.pem",
		TLSClientAuth:   "none",
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

	// Act
	err := server.Start()

	// Assert
	if err == nil {
		t.Fatal("Start() expected error when initErr is set")
	}
	if !strings.Contains(err.Error(), "server initialization") {
		t.Errorf("error = %v, want to contain 'server initialization'", err)
	}
}

func TestServer_Start_WithTLS(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	certPath, keyPath := generateTestCert(t, dir)

	cfg := &config.Config{
		ServerPort:      0, // Use port 0 for random available port
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 5 * time.Second,
		TLSEnabled:      true,
		TLSCertPath:     certPath,
		TLSKeyPath:      keyPath,
		TLSClientAuth:   "none",
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

	// Act - Start server in background and immediately shut down
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)

	// Assert - Start should return nil (ErrServerClosed is swallowed)
	err := <-errCh
	if err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
}

func TestNew_WithAuthenticator(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	authenticator := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}

	// Act
	server := New(cfg, logger, itemStore, authenticator)

	// Assert
	if server == nil {
		t.Fatal("New() returned nil")
	}
	if server.authenticator == nil {
		t.Error("authenticator should not be nil")
	}

	// Verify auth middleware is applied - protected endpoint should return 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Protected endpoint status = %d, want %d when auth fails", rr.Code, http.StatusUnauthorized)
	}

	// Health endpoint should still be accessible (public path)
	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	rr = httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Health endpoint status = %d, want %d (public path)", rr.Code, http.StatusOK)
	}
}

func TestNew_WithProbeServer(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

	// Assert
	if server.probeServer == nil {
		t.Error("probeServer should not be nil when ProbePort > 0")
	}
	if server.probeRouter == nil {
		t.Error("probeRouter should not be nil")
	}
	if server.probeServer.Addr != ":9090" {
		t.Errorf("probeServer.Addr = %s, want :9090", server.probeServer.Addr)
	}
}

func TestNew_ProbeServerDisabled(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       0,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

	// Assert
	if server.probeServer != nil {
		t.Error("probeServer should be nil when ProbePort == 0")
	}
	// probeRouter should still be created (for testing)
	if server.probeRouter == nil {
		t.Error("probeRouter should not be nil even when ProbePort == 0")
	}
}

func TestServer_ProbeHealthEndpoint(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	// Act
	server.probeRouter.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Probe health endpoint status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestServer_ProbeReadyEndpoint(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()

	// Act
	server.probeRouter.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Probe ready endpoint status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestServer_ProbeMetricsEndpoint(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	// Act
	server.probeRouter.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Probe metrics endpoint status = %d, want %d when metrics enabled", rr.Code, http.StatusOK)
	}
}

func TestServer_ProbeMetricsDisabled(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  false,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	// Act
	server.probeRouter.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Probe metrics endpoint status = %d, want 404 or 405 when metrics disabled", rr.Code)
	}
}

func TestServer_ProbeNoAuth(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	authenticator := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	server := New(cfg, logger, itemStore, authenticator)

	// Act - Probe health should work without auth
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	server.probeRouter.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Probe health should be accessible without auth, got status %d", rr.Code)
	}

	// Act - Probe ready should work without auth
	req = httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr = httptest.NewRecorder()
	server.probeRouter.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Probe ready should be accessible without auth, got status %d", rr.Code)
	}

	// Act - Probe metrics should work without auth
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr = httptest.NewRecorder()
	server.probeRouter.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Probe metrics should be accessible without auth, got status %d", rr.Code)
	}
}

func TestServer_ProbeRouter(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()
	server := New(cfg, logger, itemStore, nil)

	// Act
	probeRouter := server.ProbeRouter()

	// Assert
	if probeRouter == nil {
		t.Error("ProbeRouter() returned nil")
	}
	if probeRouter != server.probeRouter {
		t.Error("ProbeRouter() should return the server's probe router")
	}
}

func TestServer_ProbeServerConfiguration(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		ServerPort:      8080,
		ProbePort:       9090,
		LogLevel:        "info",
		ShutdownTimeout: 30 * time.Second,
		MetricsEnabled:  true,
	}
	logger := zap.NewNop()
	itemStore := store.NewMemoryStore()

	// Act
	server := New(cfg, logger, itemStore, nil)

	// Assert
	if server.probeServer.ReadTimeout != 5*time.Second {
		t.Errorf("probeServer.ReadTimeout = %v, want 5s", server.probeServer.ReadTimeout)
	}
	if server.probeServer.ReadHeaderTimeout != 2*time.Second {
		t.Errorf("probeServer.ReadHeaderTimeout = %v, want 2s", server.probeServer.ReadHeaderTimeout)
	}
	if server.probeServer.WriteTimeout != 5*time.Second {
		t.Errorf("probeServer.WriteTimeout = %v, want 5s", server.probeServer.WriteTimeout)
	}
	if server.probeServer.IdleTimeout != 30*time.Second {
		t.Errorf("probeServer.IdleTimeout = %v, want 30s", server.probeServer.IdleTimeout)
	}
	if server.probeServer.MaxHeaderBytes != 1<<20 {
		t.Errorf("probeServer.MaxHeaderBytes = %d, want %d", server.probeServer.MaxHeaderBytes, 1<<20)
	}
}
