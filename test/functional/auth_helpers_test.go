//go:build functional

package functional

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/server"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// pathExists reports whether the given path exists on disk.
func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// funcCertsDir resolves the Vault-issued certificate directory for functional
// mTLS tests. It honours CERTS_DIR when present and existing, otherwise falls
// back to the repo-relative docker-compose certs directory.
func funcCertsDir() string {
	if dir := os.Getenv("CERTS_DIR"); dir != "" {
		if pathExists(dir) {
			return dir
		}
		alt := filepath.Join("../docker-compose", filepath.Base(dir))
		if pathExists(alt) {
			return alt
		}
	}
	return "../docker-compose/certs"
}

// NewMetricsTestServer creates a metrics-enabled API key server so that the
// /metrics endpoint and the extended metric set can be exercised in-process.
func NewMetricsTestServer(t *testing.T, apiKeys string) *TestServer {
	t.Helper()

	testCfg := LoadTestConfig()

	listener, err := net.Listen(
		"tcp", fmt.Sprintf("%s:%d", testCfg.Host, testCfg.Port),
	)
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	cfg := &config.Config{
		ServerPort:      port,
		ProbePort:       0,
		LogLevel:        testCfg.LogLevel,
		ShutdownTimeout: DefaultShutdownTimeout,
		MetricsEnabled:  true, // force metrics on for assertion
		AuthMode:        "apikey",
		APIKeys:         apiKeys,
	}

	authenticator, err := auth.NewAPIKeyAuthenticator(apiKeys)
	if err != nil {
		t.Fatalf("Failed to create API key authenticator: %v", err)
	}

	// Instrument the store so store_operations_total is populated.
	itemStore := store.NewInstrumentedStore(store.NewMemoryStore())
	srv := server.New(cfg, zap.NewNop(), itemStore, authenticator)

	return &TestServer{
		Server:   srv,
		Store:    store.NewMemoryStore(),
		BaseURL:  fmt.Sprintf("http://%s:%d", testCfg.Host, port),
		WSURL:    fmt.Sprintf("ws://%s:%d", testCfg.Host, port),
		Port:     port,
		listener: listener,
		t:        t,
	}
}

// mtlsTestServer holds an in-process TLS server used by functional mTLS tests.
type mtlsTestServer struct {
	baseURL string
	srv     *server.Server
	cleanup func()
}

// NewFunctionalMTLSServer starts an in-process TLS server with
// client-auth=require backed by the Vault-issued server certificate and an
// mTLS authenticator. It returns nil and skips if the certs are unavailable.
func NewFunctionalMTLSServer(t *testing.T) *mtlsTestServer {
	t.Helper()

	dir := funcCertsDir()
	caCert := filepath.Join(dir, "ca-cert.pem")
	serverCert := filepath.Join(dir, "server-cert.pem")
	serverKey := filepath.Join(dir, "server-key.pem")

	for _, p := range []string{caCert, serverCert, serverKey} {
		if !pathExists(p) {
			t.Skipf("mTLS cert %s unavailable; skipping functional mTLS", p)
		}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	cfg := &config.Config{
		ServerPort:      port,
		ProbePort:       0,
		LogLevel:        "error",
		ShutdownTimeout: DefaultShutdownTimeout,
		MetricsEnabled:  false,
		AuthMode:        "mtls",
		TLSEnabled:      true,
		TLSCertPath:     serverCert,
		TLSKeyPath:      serverKey,
		TLSCAPath:       caCert,
		TLSClientAuth:   "require",
	}

	itemStore := store.NewMemoryStore()
	srv := server.New(cfg, zap.NewNop(), itemStore, auth.NewMTLSAuthenticator())

	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			t.Logf("mTLS server exited: %v", err)
		}
	}()

	mt := &mtlsTestServer{
		baseURL: fmt.Sprintf("https://127.0.0.1:%d", port),
		srv:     srv,
		cleanup: func() {
			ctx, cancel := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer cancel()
			_ = srv.Shutdown(ctx)
		},
	}

	mt.waitReady(t, caCert, dir)
	t.Cleanup(mt.cleanup)
	return mt
}

// waitReady polls /health over mTLS until the server responds.
func (mt *mtlsTestServer) waitReady(t *testing.T, caCert, dir string) {
	t.Helper()

	client, err := funcTLSClient(
		caCert,
		filepath.Join(dir, "test-client-cert.pem"),
		filepath.Join(dir, "test-client-key.pem"),
	)
	if err != nil {
		t.Fatalf("Failed to build mTLS readiness client: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	delay := 25 * time.Millisecond
	for time.Now().Before(deadline) {
		resp, gerr := client.Get(mt.baseURL + "/health")
		if gerr == nil {
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
	t.Fatalf("mTLS server did not become ready in time")
}

// funcTLSClient builds an mTLS-capable HTTP client for functional tests.
func funcTLSClient(caCert, clientCert, clientKey string) (*http.Client, error) {
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
		Timeout: DefaultRequestTimeout,
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

// funcNoCertTLSClient builds an HTTPS client that trusts the CA but presents
// no client certificate (used to assert handshake rejection).
func funcNoCertTLSClient(caCert string) (*http.Client, error) {
	caPEM, err := os.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("reading CA cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to append CA cert to pool")
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
	}, nil
}
