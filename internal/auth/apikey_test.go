package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

func TestNewAPIKeyAuthenticator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name:    "valid single key config",
			config:  "secret-key-123:service-a",
			wantErr: false,
		},
		{
			name:    "empty config returns error",
			config:  "",
			wantErr: true,
		},
		{
			name:    "whitespace-only config returns error",
			config:  "   ",
			wantErr: true,
		},
		{
			name:    "invalid format no colon returns error",
			config:  "keywithoutnamepart",
			wantErr: true,
		},
		{
			name:    "multiple keys all parsed",
			config:  "key1:name1,key2:name2,key3:name3",
			wantErr: false,
		},
		{
			name:    "empty key returns error",
			config:  ":somename",
			wantErr: true,
		},
		{
			name:    "empty name returns error",
			config:  "somekey:",
			wantErr: true,
		},
		{
			name:    "config with trailing comma",
			config:  "key1:name1,",
			wantErr: false,
		},
		{
			name:    "config with spaces around entries",
			config:  " key1:name1 , key2:name2 ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			authenticator, err := auth.NewAPIKeyAuthenticator(tt.config)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Error("NewAPIKeyAuthenticator() error = nil, want error")
				}
				if authenticator != nil {
					t.Error("NewAPIKeyAuthenticator() returned non-nil on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewAPIKeyAuthenticator() error = %v, want nil", err)
				}
				if authenticator == nil {
					t.Error("NewAPIKeyAuthenticator() returned nil, want non-nil")
				}
			}
		})
	}
}

func TestAPIKeyAuthenticator_Authenticate(t *testing.T) {
	t.Parallel()

	authenticator, err := auth.NewAPIKeyAuthenticator(
		"valid-key-123:service-alpha,another-key:service-beta",
	)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	tests := []struct {
		name        string
		setupReq    func() *http.Request
		wantSubject string
		wantErr     bool
		wantErrIs   error
	}{
		{
			name: "no X-API-Key header returns ErrUnauthenticated",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name: "invalid key returns ErrInvalidAPIKey",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("X-API-Key", "wrong-key")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrInvalidAPIKey,
		},
		{
			name: "valid key returns AuthInfo with key name",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("X-API-Key", "valid-key-123")
				return req
			},
			wantErr:     false,
			wantSubject: "service-alpha",
		},
		{
			name: "second valid key returns correct name",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("X-API-Key", "another-key")
				return req
			},
			wantErr:     false,
			wantSubject: "service-beta",
		},
		{
			name: "empty X-API-Key header returns ErrUnauthenticated",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("X-API-Key", "")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			req := tt.setupReq()

			// Act
			info, authErr := authenticator.Authenticate(req)

			// Assert
			if tt.wantErr {
				if authErr == nil {
					t.Fatal("Authenticate() error = nil, want error")
				}
				if tt.wantErrIs != nil && !errors.Is(authErr, tt.wantErrIs) {
					t.Errorf(
						"Authenticate() error = %v, want errors.Is %v",
						authErr, tt.wantErrIs,
					)
				}
				return
			}

			if authErr != nil {
				t.Fatalf("Authenticate() error = %v, want nil", authErr)
			}
			if info == nil {
				t.Fatal("Authenticate() returned nil AuthInfo")
			}
			if info.Method != auth.AuthMethodAPIKey {
				t.Errorf("Method = %q, want %q", info.Method, auth.AuthMethodAPIKey)
			}
			if info.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", info.Subject, tt.wantSubject)
			}
		})
	}
}

func TestAPIKeyAuthenticator_Method(t *testing.T) {
	t.Parallel()

	authenticator, err := auth.NewAPIKeyAuthenticator("key:name")
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	if authenticator.Method() != auth.AuthMethodAPIKey {
		t.Errorf(
			"Method() = %q, want %q",
			authenticator.Method(), auth.AuthMethodAPIKey,
		)
	}
}

func TestAPIKeyHeader_Constant(t *testing.T) {
	t.Parallel()

	if auth.APIKeyHeader != "X-API-Key" {
		t.Errorf("APIKeyHeader = %q, want %q", auth.APIKeyHeader, "X-API-Key")
	}
}

func TestAPIKeyAuthenticator_FullIteration(t *testing.T) {
	t.Parallel()

	// Arrange - Create authenticator with multiple keys
	// The timing fix ensures all keys are always compared
	authenticator, err := auth.NewAPIKeyAuthenticator(
		"key-alpha:service-a,key-beta:service-b,key-gamma:service-c",
	)
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator() error = %v", err)
	}

	tests := []struct {
		name        string
		apiKey      string
		wantSubject string
		wantErr     bool
		wantErrIs   error
	}{
		{
			name:        "first key matches",
			apiKey:      "key-alpha",
			wantSubject: "service-a",
		},
		{
			name:        "middle key matches",
			apiKey:      "key-beta",
			wantSubject: "service-b",
		},
		{
			name:        "last key matches",
			apiKey:      "key-gamma",
			wantSubject: "service-c",
		},
		{
			name:      "no key matches",
			apiKey:    "key-delta",
			wantErr:   true,
			wantErrIs: auth.ErrInvalidAPIKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-API-Key", tt.apiKey)

			info, authErr := authenticator.Authenticate(req)

			if tt.wantErr {
				if authErr == nil {
					t.Fatal("Authenticate() error = nil, want error")
				}
				if tt.wantErrIs != nil && !errors.Is(authErr, tt.wantErrIs) {
					t.Errorf("Authenticate() error = %v, want errors.Is %v", authErr, tt.wantErrIs)
				}
				return
			}

			if authErr != nil {
				t.Fatalf("Authenticate() error = %v, want nil", authErr)
			}
			if info.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", info.Subject, tt.wantSubject)
			}
		})
	}
}
