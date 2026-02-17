package auth_test

import (
	"context"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

func TestAuthMethodConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   auth.AuthMethod
		expected string
	}{
		{"AuthMethodNone", auth.AuthMethodNone, "none"},
		{"AuthMethodMTLS", auth.AuthMethodMTLS, "mtls"},
		{"AuthMethodOIDC", auth.AuthMethodOIDC, "oidc"},
		{"AuthMethodBasic", auth.AuthMethodBasic, "basic"},
		{"AuthMethodAPIKey", auth.AuthMethodAPIKey, "apikey"},
		{"AuthMethodMulti", auth.AuthMethodMulti, "multi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if string(tt.method) != tt.expected {
				t.Errorf("AuthMethod = %q, want %q", tt.method, tt.expected)
			}
		})
	}
}

func TestAuthInfoCreation(t *testing.T) {
	t.Parallel()

	// Arrange
	claims := map[string]any{
		"role":  "admin",
		"scope": "read:write",
	}

	// Act
	info := &auth.AuthInfo{
		Method:  auth.AuthMethodOIDC,
		Subject: "user@example.com",
		Claims:  claims,
	}

	// Assert
	if info.Method != auth.AuthMethodOIDC {
		t.Errorf("Method = %q, want %q", info.Method, auth.AuthMethodOIDC)
	}
	if info.Subject != "user@example.com" {
		t.Errorf("Subject = %q, want %q", info.Subject, "user@example.com")
	}
	if info.Claims["role"] != "admin" {
		t.Errorf("Claims[role] = %v, want %q", info.Claims["role"], "admin")
	}
	if info.Claims["scope"] != "read:write" {
		t.Errorf("Claims[scope] = %v, want %q", info.Claims["scope"], "read:write")
	}
}

func TestWithAuthInfoAndFromContext(t *testing.T) {
	t.Parallel()

	// Arrange
	info := &auth.AuthInfo{
		Method:  auth.AuthMethodBasic,
		Subject: "testuser",
		Claims:  map[string]any{"key": "value"},
	}
	ctx := context.Background()

	// Act
	ctx = auth.WithAuthInfo(ctx, info)
	retrieved, ok := auth.FromContext(ctx)

	// Assert
	if !ok {
		t.Fatal("FromContext() returned false, want true")
	}
	if retrieved.Method != info.Method {
		t.Errorf("Method = %q, want %q", retrieved.Method, info.Method)
	}
	if retrieved.Subject != info.Subject {
		t.Errorf("Subject = %q, want %q", retrieved.Subject, info.Subject)
	}
	if retrieved.Claims["key"] != "value" {
		t.Errorf("Claims[key] = %v, want %q", retrieved.Claims["key"], "value")
	}
}

func TestFromContext_EmptyContext(t *testing.T) {
	t.Parallel()

	// Arrange
	ctx := context.Background()

	// Act
	info, ok := auth.FromContext(ctx)

	// Assert
	if ok {
		t.Error("FromContext() returned true for empty context, want false")
	}
	if info != nil {
		t.Errorf("FromContext() returned %v, want nil", info)
	}
}

func TestFromContext_WrongKeyType(t *testing.T) {
	t.Parallel()

	// Arrange - store a value with a different key type
	type otherKey string
	ctx := context.WithValue(context.Background(), otherKey("auth_info"), "not-auth-info")

	// Act
	info, ok := auth.FromContext(ctx)

	// Assert
	if ok {
		t.Error("FromContext() returned true for wrong key type, want false")
	}
	if info != nil {
		t.Errorf("FromContext() returned %v, want nil", info)
	}
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		message string
	}{
		{
			name:    "ErrUnauthenticated",
			err:     auth.ErrUnauthenticated,
			message: "unauthenticated: no credentials provided",
		},
		{
			name:    "ErrInvalidToken",
			err:     auth.ErrInvalidToken,
			message: "invalid token",
		},
		{
			name:    "ErrInvalidCert",
			err:     auth.ErrInvalidCert,
			message: "invalid client certificate",
		},
		{
			name:    "ErrInvalidAPIKey",
			err:     auth.ErrInvalidAPIKey,
			message: "invalid API key",
		},
		{
			name:    "ErrInvalidCredentials",
			err:     auth.ErrInvalidCredentials,
			message: "invalid credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.err.Error() != tt.message {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.message)
			}
		})
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	t.Parallel()

	errs := []error{
		auth.ErrUnauthenticated,
		auth.ErrInvalidToken,
		auth.ErrInvalidCert,
		auth.ErrInvalidAPIKey,
		auth.ErrInvalidCredentials,
	}

	for i := 0; i < len(errs); i++ {
		for j := i + 1; j < len(errs); j++ {
			if errs[i] == errs[j] {
				t.Errorf("errors[%d] and errors[%d] should be distinct", i, j)
			}
		}
	}
}
