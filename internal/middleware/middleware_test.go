// Package middleware provides HTTP middleware functions for the REST API.
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
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
