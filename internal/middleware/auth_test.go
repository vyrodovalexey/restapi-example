package middleware_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/middleware"
)

// testAuthenticator is a mock authenticator for middleware tests.
type testAuthenticator struct {
	info   *auth.AuthInfo
	err    error
	method auth.AuthMethod
}

func (a *testAuthenticator) Authenticate(
	_ *http.Request,
) (*auth.AuthInfo, error) {
	return a.info, a.err
}

func (a *testAuthenticator) Method() auth.AuthMethod {
	return a.method
}

// successHandler is a simple handler that writes 200 OK.
func successHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}

// contextCheckHandler verifies AuthInfo is in the context.
func contextCheckHandler(t *testing.T) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := auth.FromContext(r.Context())
		if !ok {
			t.Error("AuthInfo not found in context")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}
		if info == nil {
			t.Error("AuthInfo is nil in context")
			w.WriteHeader(http.StatusInternalServerError)

			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("authenticated: " + info.Subject))
	})
}

func TestAuth_PublicPaths(t *testing.T) {
	t.Parallel()

	// Authenticator that always fails - public paths should bypass it
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	tests := []struct {
		name string
		path string
	}{
		{"health endpoint bypasses auth", "/health"},
		{"ready endpoint bypasses auth", "/ready"},
		{"metrics endpoint bypasses auth", "/metrics"},
		{"health subpath bypasses auth", "/health/live"},
		{"metrics subpath bypasses auth", "/metrics/prometheus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			authMiddleware := middleware.Auth(failAuth, logger)
			handler := authMiddleware(successHandler())

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			// Act
			handler.ServeHTTP(rr, req)

			// Assert
			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want %d for path %s",
					rr.Code, http.StatusOK, tt.path)
			}
		})
	}
}

func TestAuth_WebSocketUpgrade(t *testing.T) {
	t.Parallel()

	// Arrange
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d for WebSocket upgrade",
			rr.Code, http.StatusOK)
	}
}

func TestAuth_WebSocketUpgrade_CaseInsensitive(t *testing.T) {
	t.Parallel()

	// Arrange
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Upgrade", "WebSocket")
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d for WebSocket upgrade",
			rr.Code, http.StatusOK)
	}
}

func TestAuth_OptionsRequestBypassesAuth(t *testing.T) {
	t.Parallel()

	// Arrange
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/items", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d for OPTIONS request",
			rr.Code, http.StatusOK)
	}
}

func TestAuth_ValidAuth_PassesThrough(t *testing.T) {
	t.Parallel()

	// Arrange
	successAuth := &testAuthenticator{
		info: &auth.AuthInfo{
			Method:  auth.AuthMethodBasic,
			Subject: "testuser",
			Claims:  map[string]any{"role": "admin"},
		},
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(successAuth, logger)
	handler := authMiddleware(contextCheckHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != "authenticated: testuser" {
		t.Errorf("body = %q, want %q",
			rr.Body.String(), "authenticated: testuser")
	}
}

func TestAuth_NoAuth_Returns401(t *testing.T) {
	t.Parallel()

	// Arrange
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuth_InvalidAuth_Returns401(t *testing.T) {
	t.Parallel()

	// Arrange
	invalidAuth := &testAuthenticator{
		err:    auth.ErrInvalidToken,
		method: auth.AuthMethodOIDC,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(invalidAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuth_401Response_HasJSONBody(t *testing.T) {
	t.Parallel()

	// Arrange
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q",
			contentType, "application/json")
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON body: %v", err)
	}

	code, ok := body["code"].(float64)
	if !ok || int(code) != http.StatusUnauthorized {
		t.Errorf("body.code = %v, want %d", body["code"], http.StatusUnauthorized)
	}

	msg, ok := body["message"].(string)
	if !ok || msg == "" {
		t.Errorf("body.message = %v, want non-empty string", body["message"])
	}
}

func TestAuth_401Response_HasWWWAuthenticateHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		authErr   error
		wantValue string
	}{
		{
			name:      "ErrUnauthenticated sets Bearer Basic API-Key",
			authErr:   auth.ErrUnauthenticated,
			wantValue: "Bearer, Basic, API-Key",
		},
		{
			name:      "ErrInvalidToken sets Bearer error",
			authErr:   auth.ErrInvalidToken,
			wantValue: `Bearer error="invalid_token"`,
		},
		{
			name:      "ErrInvalidCredentials sets Basic realm",
			authErr:   auth.ErrInvalidCredentials,
			wantValue: `Basic realm="restapi"`,
		},
		{
			name:      "ErrInvalidAPIKey sets API-Key",
			authErr:   auth.ErrInvalidAPIKey,
			wantValue: "API-Key",
		},
		{
			name:      "ErrInvalidCert sets mTLS",
			authErr:   auth.ErrInvalidCert,
			wantValue: "mTLS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			failAuth := &testAuthenticator{
				err:    tt.authErr,
				method: auth.AuthMethodBasic,
			}
			logger := zap.NewNop()

			authMiddleware := middleware.Auth(failAuth, logger)
			handler := authMiddleware(successHandler())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
			rr := httptest.NewRecorder()

			// Act
			handler.ServeHTTP(rr, req)

			// Assert
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d",
					rr.Code, http.StatusUnauthorized)
			}

			wwwAuth := rr.Header().Get("WWW-Authenticate")
			if wwwAuth != tt.wantValue {
				t.Errorf("WWW-Authenticate = %q, want %q",
					wwwAuth, tt.wantValue)
			}
		})
	}
}

func TestAuth_401Response_WrappedErrors(t *testing.T) {
	t.Parallel()

	// Test that wrapped errors are correctly identified
	wrappedErr := errors.Join(auth.ErrInvalidToken, errors.New("token expired"))

	failAuth := &testAuthenticator{
		err:    wrappedErr,
		method: auth.AuthMethodOIDC,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuth_HandlerNotCalledOn401(t *testing.T) {
	t.Parallel()

	// Arrange
	handlerCalled := false
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(innerHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert
	if handlerCalled {
		t.Error("inner handler should NOT be called when auth fails")
	}
}

func TestAuth_HealthcheckDoesNotBypassAuth(t *testing.T) {
	t.Parallel()

	// Arrange - /healthcheck is NOT a public path (only /health is)
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert - /healthcheck should NOT bypass auth
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for /healthcheck (not a public path)",
			rr.Code, http.StatusUnauthorized)
	}
}

func TestAuth_HealthLiveSubpathBypassesAuth(t *testing.T) {
	t.Parallel()

	// Arrange - /health/live is a sub-path of /health and should bypass auth
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert - /health/live should bypass auth
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d for /health/live (sub-path of public path)",
			rr.Code, http.StatusOK)
	}
}

func TestAuth_HealthXXXDoesNotBypassAuth(t *testing.T) {
	t.Parallel()

	// Arrange - /healthXXX shares prefix but is NOT a sub-path
	failAuth := &testAuthenticator{
		err:    auth.ErrUnauthenticated,
		method: auth.AuthMethodBasic,
	}
	logger := zap.NewNop()

	authMiddleware := middleware.Auth(failAuth, logger)
	handler := authMiddleware(successHandler())

	req := httptest.NewRequest(http.MethodGet, "/healthXXX", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(rr, req)

	// Assert - /healthXXX should NOT bypass auth
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for /healthXXX (not a public path)",
			rr.Code, http.StatusUnauthorized)
	}
}
