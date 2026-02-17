// Package middleware provides HTTP middleware functions for the REST API.
package middleware

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewResponseWriter(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()

	// Act
	rw := newResponseWriter(w)

	// Assert
	if rw == nil {
		t.Fatal("newResponseWriter() returned nil")
	}
	if rw.statusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusOK)
	}
	if rw.written {
		t.Error("written should be false initially")
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"BadRequest", http.StatusBadRequest},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			w := httptest.NewRecorder()
			rw := newResponseWriter(w)

			// Act
			rw.WriteHeader(tt.statusCode)

			// Assert
			if rw.statusCode != tt.statusCode {
				t.Errorf("statusCode = %d, want %d", rw.statusCode, tt.statusCode)
			}
			if !rw.written {
				t.Error("written should be true after WriteHeader")
			}
		})
	}
}

func TestResponseWriter_WriteHeader_OnlyOnce(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	rw := newResponseWriter(w)

	// Act - Write header twice
	rw.WriteHeader(http.StatusCreated)
	rw.WriteHeader(http.StatusBadRequest) // Should be ignored

	// Assert
	if rw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusCreated)
	}
}

func TestResponseWriter_Write(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	rw := newResponseWriter(w)
	body := []byte("test body")

	// Act
	n, err := rw.Write(body)

	// Assert
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(body) {
		t.Errorf("Write() returned %d, want %d", n, len(body))
	}
	if !rw.written {
		t.Error("written should be true after Write")
	}
	if rw.statusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want %d (default)", rw.statusCode, http.StatusOK)
	}
}

func TestResponseWriter_Write_AfterWriteHeader(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	rw := newResponseWriter(w)
	body := []byte("test body")

	// Act
	rw.WriteHeader(http.StatusCreated)
	n, err := rw.Write(body)

	// Assert
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(body) {
		t.Errorf("Write() returned %d, want %d", n, len(body))
	}
	if rw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusCreated)
	}
}

func TestChain(t *testing.T) {
	// Arrange
	var order []string

	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	})

	// Act
	chain := Chain(middleware1, middleware2)
	wrapped := chain(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	// Assert
	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(order), len(expected))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %s, want %s", i, order[i], v)
		}
	}
}

func TestChain_Empty(t *testing.T) {
	// Arrange
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Act
	chain := Chain()
	wrapped := chain(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestLogging(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	middleware := Logging(logger)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("User-Agent", "test-agent")
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestLogging_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"BadRequest", http.StatusBadRequest},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			logger := zap.NewNop()
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			middleware := Logging(logger)
			wrapped := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()

			// Act
			wrapped.ServeHTTP(rr, req)

			// Assert
			if rr.Code != tt.statusCode {
				t.Errorf("status = %d, want %d", rr.Code, tt.statusCode)
			}
		})
	}
}

func TestRecovery(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// Normal handler - no panic
	})

	middleware := Recovery(logger)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert - Should complete normally
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRecovery_RecoversPanic(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})

	middleware := Recovery(logger)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Act - Should not panic
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rr.Body.String(), "Internal Server Error") {
		t.Errorf("body = %s, want to contain 'Internal Server Error'", rr.Body.String())
	}
}

func TestRecovery_RecoversPanicWithError(t *testing.T) {
	// Arrange
	logger := zap.NewNop()
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("custom error message")
	})

	middleware := Recovery(logger)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestRequestID(t *testing.T) {
	// Arrange
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that request ID is set in request header
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			t.Error("Request ID should be set in request header")
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID()
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	responseID := rr.Header().Get(RequestIDHeader)
	if responseID == "" {
		t.Error("Request ID should be set in response header")
	}
}

func TestRequestID_ExistingID(t *testing.T) {
	// Arrange
	existingID := "existing-request-id-123"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID != existingID {
			t.Errorf("Request ID = %s, want %s", requestID, existingID)
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID()
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, existingID)
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	responseID := rr.Header().Get(RequestIDHeader)
	if responseID != existingID {
		t.Errorf("Response Request ID = %s, want %s", responseID, existingID)
	}
}

func TestRequestID_GeneratesUniqueIDs(t *testing.T) {
	// Arrange
	middleware := RequestID()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := middleware(handler)

	ids := make(map[string]bool)
	numRequests := 100

	// Act
	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)

		id := rr.Header().Get(RequestIDHeader)
		if ids[id] {
			t.Errorf("Duplicate request ID generated: %s", id)
		}
		ids[id] = true
	}

	// Assert
	if len(ids) != numRequests {
		t.Errorf("Generated %d unique IDs, want %d", len(ids), numRequests)
	}
}

func TestMetrics(t *testing.T) {
	// Arrange
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := Metrics()
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestMetrics_DifferentStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"BadRequest", http.StatusBadRequest},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			middleware := Metrics()
			wrapped := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()

			// Act
			wrapped.ServeHTTP(rr, req)

			// Assert
			if rr.Code != tt.statusCode {
				t.Errorf("status = %d, want %d", rr.Code, tt.statusCode)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	// Arrange
	allowedOrigins := []string{"http://localhost:3000", "http://example.com"}
	allowedMethods := []string{"GET", "POST", "PUT", "DELETE"}
	allowedHeaders := []string{"Content-Type", "Authorization"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := CORS(allowedOrigins, allowedMethods, allowedHeaders)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:3000" {
		t.Errorf("Access-Control-Allow-Origin = %s, want http://localhost:3000", allowOrigin)
	}

	allowMethods := rr.Header().Get("Access-Control-Allow-Methods")
	if allowMethods == "" {
		t.Error("Access-Control-Allow-Methods should be set")
	}

	allowHeadersResp := rr.Header().Get("Access-Control-Allow-Headers")
	if allowHeadersResp == "" {
		t.Error("Access-Control-Allow-Headers should be set")
	}

	allowCredentials := rr.Header().Get("Access-Control-Allow-Credentials")
	if allowCredentials != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %s, want true", allowCredentials)
	}

	maxAge := rr.Header().Get("Access-Control-Max-Age")
	if maxAge != "86400" {
		t.Errorf("Access-Control-Max-Age = %s, want 86400", maxAge)
	}
}

func TestCORS_Wildcard(t *testing.T) {
	// Arrange
	allowedOrigins := []string{"*"}
	allowedMethods := []string{"GET", "POST"}
	allowedHeaders := []string{"Content-Type"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := CORS(allowedOrigins, allowedMethods, allowedHeaders)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://any-origin.com" {
		t.Errorf("Access-Control-Allow-Origin = %s, want http://any-origin.com", allowOrigin)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	// Arrange
	allowedOrigins := []string{"http://localhost:3000"}
	allowedMethods := []string{"GET"}
	allowedHeaders := []string{"Content-Type"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := CORS(allowedOrigins, allowedMethods, allowedHeaders)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://disallowed.com")
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	allowOrigin := rr.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "" {
		t.Errorf("Access-Control-Allow-Origin = %s, want empty for disallowed origin", allowOrigin)
	}
}

func TestCORS_PreflightRequest(t *testing.T) {
	// Arrange
	allowedOrigins := []string{"http://localhost:3000"}
	allowedMethods := []string{"GET", "POST", "PUT", "DELETE"}
	allowedHeaders := []string{"Content-Type", "Authorization"}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := CORS(allowedOrigins, allowedMethods, allowedHeaders)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if handlerCalled {
		t.Error("Handler should not be called for preflight request")
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "short path",
			path: "/api/v1/items",
			want: "/api/v1/items",
		},
		{
			name: "exact 50 chars",
			path: strings.Repeat("a", 50),
			want: strings.Repeat("a", 50),
		},
		{
			name: "long path",
			path: strings.Repeat("a", 100),
			want: strings.Repeat("a", 50),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := normalizePath(tt.path)

			// Assert
			if got != tt.want {
				t.Errorf("normalizePath() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name string
		strs []string
		want string
	}{
		{
			name: "empty",
			strs: []string{},
			want: "",
		},
		{
			name: "single",
			strs: []string{"GET"},
			want: "GET",
		},
		{
			name: "multiple",
			strs: []string{"GET", "POST", "PUT"},
			want: "GET, POST, PUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := joinStrings(tt.strs)

			// Assert
			if got != tt.want {
				t.Errorf("joinStrings() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGetRequestID(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
		want      string
	}{
		{
			name:      "with request ID",
			requestID: "test-id-123",
			want:      "test-id-123",
		},
		{
			name:      "without request ID",
			requestID: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.requestID != "" {
				req.Header.Set(RequestIDHeader, tt.requestID)
			}

			// Act
			got := getRequestID(req)

			// Assert
			if got != tt.want {
				t.Errorf("getRequestID() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRequestIDKey(t *testing.T) {
	if RequestIDKey != "request_id" {
		t.Errorf("RequestIDKey = %s, want request_id", RequestIDKey)
	}
}

func TestRequestIDHeader(t *testing.T) {
	if RequestIDHeader != "X-Request-ID" {
		t.Errorf("RequestIDHeader = %s, want X-Request-ID", RequestIDHeader)
	}
}

func TestMiddlewareChainIntegration(t *testing.T) {
	// Arrange
	logger := zap.NewNop()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is set
		if r.Header.Get(RequestIDHeader) == "" {
			t.Error("Request ID should be set by middleware")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Create middleware chain
	chain := Chain(
		Recovery(logger),
		RequestID(),
		Logging(logger),
		Metrics(),
		CORS([]string{"*"}, []string{"GET", "POST"}, []string{"Content-Type"}),
	)
	wrapped := chain(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Header().Get(RequestIDHeader) == "" {
		t.Error("Response should have Request ID header")
	}
	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Response should have CORS headers")
	}
}

// mockHijacker implements http.ResponseWriter and http.Hijacker.
type mockHijacker struct {
	http.ResponseWriter
	hijackCalled bool
}

func (m *mockHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.hijackCalled = true
	return nil, nil, nil
}

// mockFlusher implements http.ResponseWriter and http.Flusher.
type mockFlusher struct {
	http.ResponseWriter
	flushCalled bool
}

func (m *mockFlusher) Flush() {
	m.flushCalled = true
}

func TestResponseWriter_Hijack_WithHijacker(t *testing.T) {
	// Arrange
	inner := &mockHijacker{ResponseWriter: httptest.NewRecorder()}
	rw := newResponseWriter(inner)

	// Act
	_, _, err := rw.Hijack()

	// Assert
	if err != nil {
		t.Errorf("Hijack() error = %v, want nil", err)
	}
	if !inner.hijackCalled {
		t.Error("Hijack() should delegate to inner ResponseWriter")
	}
}

func TestResponseWriter_Hijack_WithoutHijacker(t *testing.T) {
	// Arrange - httptest.NewRecorder does NOT implement http.Hijacker
	rw := newResponseWriter(httptest.NewRecorder())

	// Act
	conn, buf, err := rw.Hijack()

	// Assert
	if err != http.ErrNotSupported {
		t.Errorf("Hijack() error = %v, want %v", err, http.ErrNotSupported)
	}
	if conn != nil {
		t.Error("Hijack() conn should be nil when not supported")
	}
	if buf != nil {
		t.Error("Hijack() buf should be nil when not supported")
	}
}

func TestResponseWriter_Flush_WithFlusher(t *testing.T) {
	// Arrange
	inner := &mockFlusher{ResponseWriter: httptest.NewRecorder()}
	rw := newResponseWriter(inner)

	// Act
	rw.Flush()

	// Assert
	if !inner.flushCalled {
		t.Error("Flush() should delegate to inner ResponseWriter")
	}
}

func TestResponseWriter_Flush_WithoutFlusher(t *testing.T) {
	// Arrange - use a ResponseWriter that does NOT implement http.Flusher
	rw := newResponseWriter(&nonFlusherWriter{header: make(http.Header)})

	// Act - should not panic
	rw.Flush()

	// Assert - no panic means success
}

// nonFlusherWriter is a ResponseWriter that does NOT implement http.Flusher.
type nonFlusherWriter struct {
	header http.Header
}

func (w *nonFlusherWriter) Header() http.Header         { return w.header }
func (w *nonFlusherWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *nonFlusherWriter) WriteHeader(_ int)           {}

func TestNormalizeRequestPath_WithMuxRouteTemplate(t *testing.T) {
	// Arrange
	router := mux.NewRouter()
	var capturedPath string

	router.HandleFunc("/api/v1/items/{id}", func(_ http.ResponseWriter, r *http.Request) {
		capturedPath = normalizeRequestPath(r)
	}).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items/123", nil)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if capturedPath != "/api/v1/items/{id}" {
		t.Errorf("normalizeRequestPath() = %s, want /api/v1/items/{id}", capturedPath)
	}
}

func TestNormalizeRequestPath_WithoutMuxRoute(t *testing.T) {
	// Arrange - request without mux route context
	req := httptest.NewRequest(http.MethodGet, "/api/v1/items/123", nil)

	// Act
	path := normalizeRequestPath(req)

	// Assert
	if path != "/api/v1/items/123" {
		t.Errorf("normalizeRequestPath() = %s, want /api/v1/items/123", path)
	}
}

func TestCORS_Wildcard_NoCredentials(t *testing.T) {
	// Arrange
	allowedOrigins := []string{"*"}
	allowedMethods := []string{"GET", "POST"}
	allowedHeaders := []string{"Content-Type"}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := CORS(allowedOrigins, allowedMethods, allowedHeaders)
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://any-origin.com")
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert - Wildcard should NOT set Access-Control-Allow-Credentials
	credentials := rr.Header().Get("Access-Control-Allow-Credentials")
	if credentials != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want empty for wildcard origin", credentials)
	}
}

func TestLogging_HealthPathsLoggedAtDebug(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantInfo bool // true = Info level, false = Debug level
	}{
		{"health path at debug", "/health", false},
		{"ready path at debug", "/ready", false},
		{"metrics path at debug", "/metrics", false},
		{"api path at info", "/api/v1/items", true},
		{"other path at info", "/other", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			core, logs := observer.New(zapcore.DebugLevel)
			logger := zap.New(core)

			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := Logging(logger)
			wrapped := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			// Act
			wrapped.ServeHTTP(rr, req)

			// Assert
			allLogs := logs.All()
			if len(allLogs) != 1 {
				t.Fatalf("expected 1 log entry, got %d", len(allLogs))
			}

			entry := allLogs[0]
			if tt.wantInfo {
				if entry.Level != zapcore.InfoLevel {
					t.Errorf("log level = %v, want Info for path %s", entry.Level, tt.path)
				}
			} else {
				if entry.Level != zapcore.DebugLevel {
					t.Errorf("log level = %v, want Debug for path %s", entry.Level, tt.path)
				}
			}
		})
	}
}

func TestRequestID_StoredInContext(t *testing.T) {
	// Arrange
	var contextRequestID interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = r.Context().Value(RequestIDKey)
		w.WriteHeader(http.StatusOK)
	})

	middleware := RequestID()
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Act
	wrapped.ServeHTTP(rr, req)

	// Assert
	if contextRequestID == nil {
		t.Error("Request ID should be stored in context")
	}

	requestIDStr, ok := contextRequestID.(string)
	if !ok {
		t.Fatalf("Request ID in context should be a string, got %T", contextRequestID)
	}
	if requestIDStr == "" {
		t.Error("Request ID in context should not be empty")
	}

	// Should match the response header
	responseID := rr.Header().Get(RequestIDHeader)
	if requestIDStr != responseID {
		t.Errorf("Context request ID = %s, response header = %s, should match", requestIDStr, responseID)
	}
}
