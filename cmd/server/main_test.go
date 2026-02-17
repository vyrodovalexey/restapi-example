package main

import (
	"testing"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/config"
)

func TestInitLogger(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr bool
	}{
		{"debug level", "debug", false},
		{"info level", "info", false},
		{"warn level", "warn", false},
		{"error level", "error", false},
		{"invalid level defaults to info", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			logger, err := initLogger(tt.level)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Error("initLogger() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("initLogger() error = %v", err)
			}
			if logger == nil {
				t.Error("initLogger() returned nil logger")
			}
		})
	}
}

func TestCreateAuthenticator_None(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "none",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createAuthenticator() error = %v", err)
	}
	if authenticator != nil {
		t.Error("createAuthenticator() should return nil for 'none' mode")
	}
}

func TestCreateAuthenticator_EmptyMode(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createAuthenticator() error = %v", err)
	}
	if authenticator != nil {
		t.Error("createAuthenticator() should return nil for empty mode")
	}
}

func TestCreateAuthenticator_MTLS(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "mtls",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createAuthenticator() should return non-nil for 'mtls' mode")
	}
}

func TestCreateAuthenticator_Basic(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode:       "basic",
		BasicAuthUsers: "user1:$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createAuthenticator() should return non-nil for 'basic' mode")
	}
}

func TestCreateAuthenticator_APIKey(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "apikey",
		APIKeys:  "secret-key:service-a",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createAuthenticator() should return non-nil for 'apikey' mode")
	}
}

func TestCreateAuthenticator_OIDC(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode:      "oidc",
		OIDCIssuerURL: "https://issuer.example.com",
		OIDCClientID:  "client-id",
	}
	logger := zap.NewNop()

	// Act
	_, err := createAuthenticator(cfg, logger)

	// Assert - OIDC returns error because it requires token verifier setup
	if err == nil {
		t.Error("createAuthenticator() expected error for 'oidc' mode")
	}
}

func TestCreateAuthenticator_UnknownMode(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "unknown",
	}
	logger := zap.NewNop()

	// Act
	_, err := createAuthenticator(cfg, logger)

	// Assert
	if err == nil {
		t.Error("createAuthenticator() expected error for unknown mode")
	}
}

func TestCreateMultiAuthenticator_WithMTLS(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode:      "multi",
		TLSEnabled:    true,
		TLSClientAuth: "require",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createMultiAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createMultiAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createMultiAuthenticator() should return non-nil")
	}
}

func TestCreateMultiAuthenticator_WithBasicAuth(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode:       "multi",
		BasicAuthUsers: "user1:$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createMultiAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createMultiAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createMultiAuthenticator() should return non-nil")
	}
}

func TestCreateMultiAuthenticator_WithAPIKey(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "multi",
		APIKeys:  "secret-key:service-a",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createMultiAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createMultiAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createMultiAuthenticator() should return non-nil")
	}
}

func TestCreateMultiAuthenticator_WithAllMethods(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode:       "multi",
		TLSEnabled:     true,
		TLSClientAuth:  "require",
		BasicAuthUsers: "user1:$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012",
		APIKeys:        "secret-key:service-a",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createMultiAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createMultiAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createMultiAuthenticator() should return non-nil")
	}
}

func TestCreateMultiAuthenticator_NoAuthenticators(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "multi",
	}
	logger := zap.NewNop()

	// Act
	_, err := createMultiAuthenticator(cfg, logger)

	// Assert
	if err == nil {
		t.Error("createMultiAuthenticator() expected error when no authenticators configured")
	}
}

func TestCreateMultiAuthenticator_InvalidBasicAuth(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode:       "multi",
		BasicAuthUsers: "invalid-no-colon",
	}
	logger := zap.NewNop()

	// Act
	_, err := createMultiAuthenticator(cfg, logger)

	// Assert
	if err == nil {
		t.Error("createMultiAuthenticator() expected error for invalid basic auth config")
	}
}

func TestCreateMultiAuthenticator_InvalidAPIKey(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "multi",
		APIKeys:  "invalid-no-colon",
	}
	logger := zap.NewNop()

	// Act
	_, err := createMultiAuthenticator(cfg, logger)

	// Assert
	if err == nil {
		t.Error("createMultiAuthenticator() expected error for invalid API key config")
	}
}

func TestCreateAuthenticator_Multi(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		AuthMode: "multi",
		APIKeys:  "secret-key:service-a",
	}
	logger := zap.NewNop()

	// Act
	authenticator, err := createAuthenticator(cfg, logger)

	// Assert
	if err != nil {
		t.Fatalf("createAuthenticator() error = %v", err)
	}
	if authenticator == nil {
		t.Error("createAuthenticator() should return non-nil for 'multi' mode")
	}
}
