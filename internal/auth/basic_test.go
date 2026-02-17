package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

func generateBcryptHash(t *testing.T, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password), bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("failed to generate bcrypt hash: %v", err)
	}

	return string(hash)
}

func TestNewBasicAuthenticator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    string
		wantErr   bool
		wantUsers int
	}{
		{
			name:    "valid single user config",
			config:  "user1:$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012",
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
			config:  "usernohash",
			wantErr: true,
		},
		{
			name:   "multiple users all parsed",
			config: "user1:hash1,user2:hash2,user3:hash3",
		},
		{
			name:    "empty username returns error",
			config:  ":somehash",
			wantErr: true,
		},
		{
			name:    "empty hash returns error",
			config:  "user:",
			wantErr: true,
		},
		{
			name:   "config with trailing comma",
			config: "user1:hash1,",
		},
		{
			name:   "config with spaces around entries",
			config: " user1:hash1 , user2:hash2 ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			authenticator, err := auth.NewBasicAuthenticator(tt.config)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Error("NewBasicAuthenticator() error = nil, want error")
				}
				if authenticator != nil {
					t.Error("NewBasicAuthenticator() returned non-nil on error")
				}
			} else {
				if err != nil {
					t.Errorf("NewBasicAuthenticator() error = %v, want nil", err)
				}
				if authenticator == nil {
					t.Error("NewBasicAuthenticator() returned nil, want non-nil")
				}
			}
		})
	}
}

func TestBasicAuthenticator_Authenticate(t *testing.T) {
	t.Parallel()

	password := "correctpassword"
	hash := generateBcryptHash(t, password)
	config := "testuser:" + hash

	authenticator, err := auth.NewBasicAuthenticator(config)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator() error = %v", err)
	}

	tests := []struct {
		name        string
		setupReq    func() *http.Request
		wantSubject string
		wantErr     bool
		wantErrIs   error
	}{
		{
			name: "no Authorization header returns ErrUnauthenticated",
			setupReq: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/", nil)
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name: "Bearer token returns ErrUnauthenticated",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer some-token")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name: "unknown username returns ErrInvalidCredentials",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.SetBasicAuth("unknownuser", password)
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrInvalidCredentials,
		},
		{
			name: "wrong password returns ErrInvalidCredentials",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.SetBasicAuth("testuser", "wrongpassword")
				return req
			},
			wantErr:   true,
			wantErrIs: auth.ErrInvalidCredentials,
		},
		{
			name: "correct credentials returns AuthInfo with username",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.SetBasicAuth("testuser", password)
				return req
			},
			wantErr:     false,
			wantSubject: "testuser",
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
			if info.Method != auth.AuthMethodBasic {
				t.Errorf("Method = %q, want %q", info.Method, auth.AuthMethodBasic)
			}
			if info.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", info.Subject, tt.wantSubject)
			}
		})
	}
}

func TestBasicAuthenticator_MultipleUsers(t *testing.T) {
	t.Parallel()

	// Arrange
	hash1 := generateBcryptHash(t, "pass1")
	hash2 := generateBcryptHash(t, "pass2")
	config := "alice:" + hash1 + ",bob:" + hash2

	authenticator, err := auth.NewBasicAuthenticator(config)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator() error = %v", err)
	}

	tests := []struct {
		name        string
		username    string
		password    string
		wantSubject string
	}{
		{"alice with correct password", "alice", "pass1", "alice"},
		{"bob with correct password", "bob", "pass2", "bob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetBasicAuth(tt.username, tt.password)

			info, authErr := authenticator.Authenticate(req)

			if authErr != nil {
				t.Fatalf("Authenticate() error = %v, want nil", authErr)
			}
			if info.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", info.Subject, tt.wantSubject)
			}
		})
	}
}

func TestBasicAuthenticator_Method(t *testing.T) {
	t.Parallel()

	hash := generateBcryptHash(t, "pass")
	authenticator, err := auth.NewBasicAuthenticator("user:" + hash)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator() error = %v", err)
	}

	if authenticator.Method() != auth.AuthMethodBasic {
		t.Errorf(
			"Method() = %q, want %q",
			authenticator.Method(), auth.AuthMethodBasic,
		)
	}
}

func TestBasicAuthenticator_TimingFix_SameErrorMessage(t *testing.T) {
	t.Parallel()

	// Arrange - Both "unknown user" and "wrong password" should return
	// the same error message to prevent user enumeration.
	password := "correctpassword"
	hash := generateBcryptHash(t, password)
	config := "testuser:" + hash

	authenticator, err := auth.NewBasicAuthenticator(config)
	if err != nil {
		t.Fatalf("NewBasicAuthenticator() error = %v", err)
	}

	// Act - unknown user
	reqUnknown := httptest.NewRequest(http.MethodGet, "/", nil)
	reqUnknown.SetBasicAuth("unknownuser", password)
	_, errUnknown := authenticator.Authenticate(reqUnknown)

	// Act - wrong password
	reqWrong := httptest.NewRequest(http.MethodGet, "/", nil)
	reqWrong.SetBasicAuth("testuser", "wrongpassword")
	_, errWrong := authenticator.Authenticate(reqWrong)

	// Assert - both should return ErrInvalidCredentials with same message
	if errUnknown == nil || errWrong == nil {
		t.Fatal("both should return errors")
	}
	if !errors.Is(errUnknown, auth.ErrInvalidCredentials) {
		t.Errorf("unknown user error = %v, want ErrInvalidCredentials", errUnknown)
	}
	if !errors.Is(errWrong, auth.ErrInvalidCredentials) {
		t.Errorf("wrong password error = %v, want ErrInvalidCredentials", errWrong)
	}
	if errUnknown.Error() != errWrong.Error() {
		t.Errorf(
			"error messages differ: unknown=%q, wrong=%q (should be same to prevent enumeration)",
			errUnknown.Error(), errWrong.Error(),
		)
	}
}
