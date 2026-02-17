package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

// mockTokenVerifier is a test double for auth.TokenVerifier.
type mockTokenVerifier struct {
	claims *auth.TokenClaims
	err    error
}

func (m *mockTokenVerifier) Verify(
	_ context.Context,
	_ string,
) (*auth.TokenClaims, error) {
	return m.claims, m.err
}

func TestOIDCAuthenticator_Authenticate(t *testing.T) {
	t.Parallel()

	validClaims := &auth.TokenClaims{
		Subject:  "user@example.com",
		Audience: []string{"my-api"},
		Issuer:   "https://issuer.example.com",
		Expiry:   time.Now().Add(time.Hour),
		Claims: map[string]any{
			"role": "admin",
		},
	}

	tests := []struct {
		name        string
		verifier    *mockTokenVerifier
		audience    string
		setupReq    func() *http.Request
		wantSubject string
		wantClaims  map[string]any
		wantErr     bool
		wantErrIs   error
	}{
		{
			name:     "no Authorization header returns ErrUnauthenticated",
			verifier: &mockTokenVerifier{},
			audience: "",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name:     "Basic auth header returns ErrUnauthenticated",
			verifier: &mockTokenVerifier{},
			audience: "",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name:     "Bearer without token returns ErrUnauthenticated",
			verifier: &mockTokenVerifier{},
			audience: "",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name: "invalid token returns ErrInvalidToken",
			verifier: &mockTokenVerifier{
				err: errors.New("token expired"),
			},
			audience: "",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer invalid-token")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrInvalidToken,
		},
		{
			name: "valid token returns AuthInfo with correct subject",
			verifier: &mockTokenVerifier{
				claims: validClaims,
			},
			audience: "",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				return req
			},
			wantErr:     false,
			wantSubject: "user@example.com",
			wantClaims:  map[string]any{"role": "admin"},
		},
		{
			name: "valid token populates Claims from token",
			verifier: &mockTokenVerifier{
				claims: &auth.TokenClaims{
					Subject:  "svc-account",
					Audience: []string{"api"},
					Issuer:   "https://issuer.example.com",
					Expiry:   time.Now().Add(time.Hour),
					Claims: map[string]any{
						"scope": "read write",
						"tier":  "premium",
					},
				},
			},
			audience: "",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				return req
			},
			wantErr:     false,
			wantSubject: "svc-account",
			wantClaims: map[string]any{
				"scope": "read write",
				"tier":  "premium",
			},
		},
		{
			name: "wrong audience returns ErrInvalidToken",
			verifier: &mockTokenVerifier{
				claims: &auth.TokenClaims{
					Subject:  "user@example.com",
					Audience: []string{"other-api"},
					Issuer:   "https://issuer.example.com",
					Expiry:   time.Now().Add(time.Hour),
					Claims:   map[string]any{},
				},
			},
			audience: "my-api",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrInvalidToken,
		},
		{
			name: "matching audience succeeds",
			verifier: &mockTokenVerifier{
				claims: &auth.TokenClaims{
					Subject:  "user@example.com",
					Audience: []string{"my-api", "other-api"},
					Issuer:   "https://issuer.example.com",
					Expiry:   time.Now().Add(time.Hour),
					Claims:   map[string]any{"aud_ok": true},
				},
			},
			audience: "my-api",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				return req
			},
			wantErr:     false,
			wantSubject: "user@example.com",
			wantClaims:  map[string]any{"aud_ok": true},
		},
		{
			name: "empty audience config skips audience check",
			verifier: &mockTokenVerifier{
				claims: &auth.TokenClaims{
					Subject:  "user@example.com",
					Audience: []string{"any-audience"},
					Issuer:   "https://issuer.example.com",
					Expiry:   time.Now().Add(time.Hour),
					Claims:   map[string]any{},
				},
			},
			audience: "",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				return req
			},
			wantErr:     false,
			wantSubject: "user@example.com",
			wantClaims:  map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			authenticator := auth.NewOIDCAuthenticator(tt.verifier, tt.audience)
			req := tt.setupReq()

			// Act
			info, err := authenticator.Authenticate(req)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Fatal("Authenticate() error = nil, want error")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf(
						"Authenticate() error = %v, want errors.Is %v",
						err, tt.wantErrIs,
					)
				}
				return
			}

			if err != nil {
				t.Fatalf("Authenticate() error = %v, want nil", err)
			}
			if info == nil {
				t.Fatal("Authenticate() returned nil AuthInfo")
			}
			if info.Method != auth.AuthMethodOIDC {
				t.Errorf("Method = %q, want %q", info.Method, auth.AuthMethodOIDC)
			}
			if info.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", info.Subject, tt.wantSubject)
			}
			for key, wantVal := range tt.wantClaims {
				gotVal, exists := info.Claims[key]
				if !exists {
					t.Errorf("Claims[%q] not found", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("Claims[%q] = %v, want %v", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestOIDCAuthenticator_Method(t *testing.T) {
	t.Parallel()

	verifier := &mockTokenVerifier{}
	authenticator := auth.NewOIDCAuthenticator(verifier, "")

	if authenticator.Method() != auth.AuthMethodOIDC {
		t.Errorf("Method() = %q, want %q", authenticator.Method(), auth.AuthMethodOIDC)
	}
}
