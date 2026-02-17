// Package config provides configuration management for the REST API server.
package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Arrange - Clear all environment variables
	clearEnvVars(t)

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.ServerPort != DefaultServerPort {
		t.Errorf("ServerPort = %d, want %d", cfg.ServerPort, DefaultServerPort)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %s, want %s", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, DefaultShutdownTimeout)
	}
	if cfg.MetricsEnabled != DefaultMetricsEnabled {
		t.Errorf("MetricsEnabled = %v, want %v", cfg.MetricsEnabled, DefaultMetricsEnabled)
	}
	if cfg.OTLPEndpoint != "" {
		t.Errorf("OTLPEndpoint = %s, want empty string", cfg.OTLPEndpoint)
	}
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *Config)
	}{
		{
			name: "custom server port",
			envVars: map[string]string{
				EnvServerPort: "9090",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.ServerPort != 9090 {
					t.Errorf("ServerPort = %d, want 9090", cfg.ServerPort)
				}
			},
		},
		{
			name: "custom log level",
			envVars: map[string]string{
				EnvLogLevel: "debug",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.LogLevel != "debug" {
					t.Errorf("LogLevel = %s, want debug", cfg.LogLevel)
				}
			},
		},
		{
			name: "custom shutdown timeout",
			envVars: map[string]string{
				EnvShutdownTimeout: "60s",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.ShutdownTimeout != 60*time.Second {
					t.Errorf("ShutdownTimeout = %v, want 60s", cfg.ShutdownTimeout)
				}
			},
		},
		{
			name: "metrics disabled",
			envVars: map[string]string{
				EnvMetricsEnabled: "false",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.MetricsEnabled != false {
					t.Errorf("MetricsEnabled = %v, want false", cfg.MetricsEnabled)
				}
			},
		},
		{
			name: "custom OTLP endpoint",
			envVars: map[string]string{
				EnvOTLPEndpoint: "http://localhost:4317",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.OTLPEndpoint != "http://localhost:4317" {
					t.Errorf("OTLPEndpoint = %s, want http://localhost:4317", cfg.OTLPEndpoint)
				}
			},
		},
		{
			name: "all custom values",
			envVars: map[string]string{
				EnvServerPort:      "3000",
				EnvLogLevel:        "warn",
				EnvShutdownTimeout: "45s",
				EnvMetricsEnabled:  "true",
				EnvOTLPEndpoint:    "http://otel:4317",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.ServerPort != 3000 {
					t.Errorf("ServerPort = %d, want 3000", cfg.ServerPort)
				}
				if cfg.LogLevel != "warn" {
					t.Errorf("LogLevel = %s, want warn", cfg.LogLevel)
				}
				if cfg.ShutdownTimeout != 45*time.Second {
					t.Errorf("ShutdownTimeout = %v, want 45s", cfg.ShutdownTimeout)
				}
				if cfg.MetricsEnabled != true {
					t.Errorf("MetricsEnabled = %v, want true", cfg.MetricsEnabled)
				}
				if cfg.OTLPEndpoint != "http://otel:4317" {
					t.Errorf("OTLPEndpoint = %s, want http://otel:4317", cfg.OTLPEndpoint)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			tt.validate(t, cfg)
		})
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantErr     error
		wantErrText string
	}{
		{
			name: "invalid server port - zero",
			envVars: map[string]string{
				EnvServerPort: "0",
			},
			wantErr: ErrInvalidServerPort,
		},
		{
			name: "invalid server port - negative",
			envVars: map[string]string{
				EnvServerPort: "-1",
			},
			wantErr: ErrInvalidServerPort,
		},
		{
			name: "invalid server port - too high",
			envVars: map[string]string{
				EnvServerPort: "65536",
			},
			wantErr: ErrInvalidServerPort,
		},
		{
			name: "invalid log level",
			envVars: map[string]string{
				EnvLogLevel: "invalid",
			},
			wantErr: ErrInvalidLogLevel,
		},
		{
			name: "invalid shutdown timeout - negative",
			envVars: map[string]string{
				EnvShutdownTimeout: "-1s",
			},
			wantErr: ErrInvalidShutdownTimeout,
		},
		{
			name: "invalid shutdown timeout - zero",
			envVars: map[string]string{
				EnvShutdownTimeout: "0s",
			},
			wantErr: ErrInvalidShutdownTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatalf("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
			if tt.wantErr != nil {
				if !containsError(err, tt.wantErr) {
					t.Errorf("Load() error = %v, want error containing %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestLoad_ParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
	}{
		{
			name: "invalid server port - not a number",
			envVars: map[string]string{
				EnvServerPort: "abc",
			},
		},
		{
			name: "invalid shutdown timeout - bad format",
			envVars: map[string]string{
				EnvShutdownTimeout: "invalid",
			},
		},
		{
			name: "invalid metrics enabled - not a bool",
			envVars: map[string]string{
				EnvMetricsEnabled: "notabool",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatalf("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "valid config",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				MetricsEnabled:  true,
			},
			wantErr: nil,
		},
		{
			name: "valid config - minimum port",
			config: Config{
				ServerPort:      1,
				LogLevel:        "debug",
				ShutdownTimeout: 1 * time.Second,
			},
			wantErr: nil,
		},
		{
			name: "valid config - maximum port",
			config: Config{
				ServerPort:      65535,
				LogLevel:        "error",
				ShutdownTimeout: 1 * time.Second,
			},
			wantErr: nil,
		},
		{
			name: "invalid port - zero",
			config: Config{
				ServerPort:      0,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
			},
			wantErr: ErrInvalidServerPort,
		},
		{
			name: "invalid port - negative",
			config: Config{
				ServerPort:      -1,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
			},
			wantErr: ErrInvalidServerPort,
		},
		{
			name: "invalid port - too high",
			config: Config{
				ServerPort:      65536,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
			},
			wantErr: ErrInvalidServerPort,
		},
		{
			name: "invalid log level",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "invalid",
				ShutdownTimeout: 30 * time.Second,
			},
			wantErr: ErrInvalidLogLevel,
		},
		{
			name: "invalid shutdown timeout - zero",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 0,
			},
			wantErr: ErrInvalidShutdownTimeout,
		},
		{
			name: "invalid shutdown timeout - negative",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: -1 * time.Second,
			},
			wantErr: ErrInvalidShutdownTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.config.Validate()

			// Assert
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestConfig_Address(t *testing.T) {
	tests := []struct {
		name       string
		serverPort int
		want       string
	}{
		{
			name:       "default port",
			serverPort: 8080,
			want:       ":8080",
		},
		{
			name:       "custom port",
			serverPort: 3000,
			want:       ":3000",
		},
		{
			name:       "minimum port",
			serverPort: 1,
			want:       ":1",
		},
		{
			name:       "maximum port",
			serverPort: 65535,
			want:       ":65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{ServerPort: tt.serverPort}

			// Act
			got := cfg.Address()

			// Assert
			if got != tt.want {
				t.Errorf("Address() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			// Arrange
			cfg := &Config{
				ServerPort:      8080,
				LogLevel:        level,
				ShutdownTimeout: 30 * time.Second,
			}

			// Act
			err := cfg.Validate()

			// Assert
			if err != nil {
				t.Errorf("Validate() with log level %s returned unexpected error: %v", level, err)
			}
		})
	}
}

func TestLoadAuthModeDefaults(t *testing.T) {
	// Arrange
	clearEnvVars(t)

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if cfg.AuthMode != DefaultAuthMode {
		t.Errorf("AuthMode = %s, want %s", cfg.AuthMode, DefaultAuthMode)
	}
	if cfg.TLSEnabled {
		t.Error("TLSEnabled should default to false")
	}
	if cfg.TLSClientAuth != DefaultTLSClientAuth {
		t.Errorf("TLSClientAuth = %s, want %s", cfg.TLSClientAuth, DefaultTLSClientAuth)
	}
	if cfg.VaultEnabled {
		t.Error("VaultEnabled should default to false")
	}
}

func TestLoadAuthModeValues(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		envVars  map[string]string
		wantErr  bool
		wantMode string
	}{
		{
			name:     "auth mode none",
			mode:     "none",
			envVars:  map[string]string{EnvAuthMode: "none"},
			wantMode: "none",
		},
		{
			name:     "auth mode mtls",
			mode:     "mtls",
			envVars:  map[string]string{EnvAuthMode: "mtls"},
			wantMode: "mtls",
		},
		{
			name: "auth mode oidc",
			mode: "oidc",
			envVars: map[string]string{
				EnvAuthMode:      "oidc",
				EnvOIDCIssuerURL: "https://issuer.example.com",
				EnvOIDCClientID:  "my-client",
			},
			wantMode: "oidc",
		},
		{
			name: "auth mode basic",
			mode: "basic",
			envVars: map[string]string{
				EnvAuthMode:       "basic",
				EnvBasicAuthUsers: "user1:hash1",
			},
			wantMode: "basic",
		},
		{
			name: "auth mode apikey",
			mode: "apikey",
			envVars: map[string]string{
				EnvAuthMode: "apikey",
				EnvAPIKeys:  "key1:name1",
			},
			wantMode: "apikey",
		},
		{
			name: "auth mode multi with basic auth",
			mode: "multi",
			envVars: map[string]string{
				EnvAuthMode:       "multi",
				EnvBasicAuthUsers: "user1:hash1",
			},
			wantMode: "multi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			if cfg.AuthMode != tt.wantMode {
				t.Errorf("AuthMode = %s, want %s", cfg.AuthMode, tt.wantMode)
			}
		})
	}
}

func TestLoadAuthModeInvalid(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr error
	}{
		{
			name:    "invalid auth mode - unknown",
			mode:    "unknown",
			wantErr: ErrInvalidAuthMode,
		},
		{
			name:    "invalid auth mode - empty string set explicitly",
			mode:    "foobar",
			wantErr: ErrInvalidAuthMode,
		},
		{
			name:    "invalid auth mode - uppercase",
			mode:    "BASIC",
			wantErr: ErrInvalidAuthMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			t.Setenv(EnvAuthMode, tt.mode)

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
			if !containsError(err, tt.wantErr) {
				t.Errorf("Load() error = %v, want error containing %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadTLSConfig(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *Config)
	}{
		{
			name: "TLS enabled with cert and key",
			envVars: map[string]string{
				EnvTLSEnabled:  "true",
				EnvTLSCertPath: "/path/to/cert.pem",
				EnvTLSKeyPath:  "/path/to/key.pem",
			},
			validate: func(t *testing.T, cfg *Config) {
				if !cfg.TLSEnabled {
					t.Error("TLSEnabled should be true")
				}
				if cfg.TLSCertPath != "/path/to/cert.pem" {
					t.Errorf("TLSCertPath = %s, want /path/to/cert.pem", cfg.TLSCertPath)
				}
				if cfg.TLSKeyPath != "/path/to/key.pem" {
					t.Errorf("TLSKeyPath = %s, want /path/to/key.pem", cfg.TLSKeyPath)
				}
			},
		},
		{
			name: "TLS with CA path and client auth request",
			envVars: map[string]string{
				EnvTLSEnabled:    "true",
				EnvTLSCertPath:   "/path/to/cert.pem",
				EnvTLSKeyPath:    "/path/to/key.pem",
				EnvTLSCAPath:     "/path/to/ca.pem",
				EnvTLSClientAuth: "request",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.TLSCAPath != "/path/to/ca.pem" {
					t.Errorf("TLSCAPath = %s, want /path/to/ca.pem", cfg.TLSCAPath)
				}
				if cfg.TLSClientAuth != "request" {
					t.Errorf("TLSClientAuth = %s, want request", cfg.TLSClientAuth)
				}
			},
		},
		{
			name: "TLS with client auth require and CA",
			envVars: map[string]string{
				EnvTLSEnabled:    "true",
				EnvTLSCertPath:   "/path/to/cert.pem",
				EnvTLSKeyPath:    "/path/to/key.pem",
				EnvTLSCAPath:     "/path/to/ca.pem",
				EnvTLSClientAuth: "require",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.TLSClientAuth != "require" {
					t.Errorf("TLSClientAuth = %s, want require", cfg.TLSClientAuth)
				}
			},
		},
		{
			name: "TLS disabled explicitly",
			envVars: map[string]string{
				EnvTLSEnabled: "false",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.TLSEnabled {
					t.Error("TLSEnabled should be false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			tt.validate(t, cfg)
		})
	}
}

func TestLoadTLSValidation(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr error
	}{
		{
			name: "TLS enabled without cert path",
			envVars: map[string]string{
				EnvTLSEnabled: "true",
				EnvTLSKeyPath: "/path/to/key.pem",
			},
			wantErr: ErrInvalidTLSCertRequired,
		},
		{
			name: "TLS enabled without key path",
			envVars: map[string]string{
				EnvTLSEnabled:  "true",
				EnvTLSCertPath: "/path/to/cert.pem",
			},
			wantErr: ErrInvalidTLSCertRequired,
		},
		{
			name: "TLS enabled without cert and key",
			envVars: map[string]string{
				EnvTLSEnabled: "true",
			},
			wantErr: ErrInvalidTLSCertRequired,
		},
		{
			name: "TLS client auth require without CA",
			envVars: map[string]string{
				EnvTLSEnabled:    "true",
				EnvTLSCertPath:   "/path/to/cert.pem",
				EnvTLSKeyPath:    "/path/to/key.pem",
				EnvTLSClientAuth: "require",
			},
			wantErr: ErrInvalidTLSCARequired,
		},
		{
			name: "invalid TLS client auth value",
			envVars: map[string]string{
				EnvTLSEnabled:    "true",
				EnvTLSCertPath:   "/path/to/cert.pem",
				EnvTLSKeyPath:    "/path/to/key.pem",
				EnvTLSClientAuth: "invalid",
			},
			wantErr: ErrInvalidTLSClientAuth,
		},
		{
			name: "invalid TLS enabled value",
			envVars: map[string]string{
				EnvTLSEnabled: "notabool",
			},
			wantErr: nil, // parse error, not validation error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
			if tt.wantErr != nil {
				if !containsError(err, tt.wantErr) {
					t.Errorf("Load() error = %v, want error containing %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestLoadOIDCConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr error
	}{
		{
			name: "OIDC mode without issuer URL",
			envVars: map[string]string{
				EnvAuthMode:     "oidc",
				EnvOIDCClientID: "my-client",
			},
			wantErr: ErrInvalidOIDCConfig,
		},
		{
			name: "OIDC mode without client ID",
			envVars: map[string]string{
				EnvAuthMode:      "oidc",
				EnvOIDCIssuerURL: "https://issuer.example.com",
			},
			wantErr: ErrInvalidOIDCConfig,
		},
		{
			name: "OIDC mode without both issuer and client",
			envVars: map[string]string{
				EnvAuthMode: "oidc",
			},
			wantErr: ErrInvalidOIDCConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
			if !containsError(err, tt.wantErr) {
				t.Errorf("Load() error = %v, want error containing %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadOIDCConfigValid(t *testing.T) {
	// Arrange
	clearEnvVars(t)
	t.Setenv(EnvAuthMode, "oidc")
	t.Setenv(EnvOIDCIssuerURL, "https://issuer.example.com")
	t.Setenv(EnvOIDCClientID, "my-client")
	t.Setenv(EnvOIDCAudience, "my-audience")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if cfg.OIDCIssuerURL != "https://issuer.example.com" {
		t.Errorf("OIDCIssuerURL = %s, want https://issuer.example.com", cfg.OIDCIssuerURL)
	}
	if cfg.OIDCClientID != "my-client" {
		t.Errorf("OIDCClientID = %s, want my-client", cfg.OIDCClientID)
	}
	if cfg.OIDCAudience != "my-audience" {
		t.Errorf("OIDCAudience = %s, want my-audience", cfg.OIDCAudience)
	}
}

func TestLoadBasicAuthConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr error
	}{
		{
			name: "basic mode without users config",
			envVars: map[string]string{
				EnvAuthMode: "basic",
			},
			wantErr: ErrInvalidBasicAuthConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
			if !containsError(err, tt.wantErr) {
				t.Errorf("Load() error = %v, want error containing %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadBasicAuthConfigValid(t *testing.T) {
	// Arrange
	clearEnvVars(t)
	t.Setenv(EnvAuthMode, "basic")
	t.Setenv(EnvBasicAuthUsers, "user1:$2a$04$hash1,user2:$2a$04$hash2")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if cfg.BasicAuthUsers != "user1:$2a$04$hash1,user2:$2a$04$hash2" {
		t.Errorf("BasicAuthUsers = %s, want user1:$2a$04$hash1,user2:$2a$04$hash2",
			cfg.BasicAuthUsers)
	}
}

func TestLoadAPIKeyConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr error
	}{
		{
			name: "apikey mode without keys config",
			envVars: map[string]string{
				EnvAuthMode: "apikey",
			},
			wantErr: ErrInvalidAPIKeyConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
			if !containsError(err, tt.wantErr) {
				t.Errorf("Load() error = %v, want error containing %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadAPIKeyConfigValid(t *testing.T) {
	// Arrange
	clearEnvVars(t)
	t.Setenv(EnvAuthMode, "apikey")
	t.Setenv(EnvAPIKeys, "key1:service1,key2:service2")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if cfg.APIKeys != "key1:service1,key2:service2" {
		t.Errorf("APIKeys = %s, want key1:service1,key2:service2", cfg.APIKeys)
	}
}

func TestLoadMultiAuthConfig(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr error
	}{
		{
			name: "multi mode without any auth config",
			envVars: map[string]string{
				EnvAuthMode: "multi",
			},
			wantErr: ErrInvalidMultiAuthConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err == nil {
				t.Fatal("Load() expected error, got nil")
			}
			if cfg != nil {
				t.Errorf("Load() expected nil config on error, got %+v", cfg)
			}
			if !containsError(err, tt.wantErr) {
				t.Errorf("Load() error = %v, want error containing %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMultiAuthConfigValid(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
	}{
		{
			name: "multi mode with basic auth users",
			envVars: map[string]string{
				EnvAuthMode:       "multi",
				EnvBasicAuthUsers: "user1:hash1",
			},
		},
		{
			name: "multi mode with API keys",
			envVars: map[string]string{
				EnvAuthMode: "multi",
				EnvAPIKeys:  "key1:name1",
			},
		},
		{
			name: "multi mode with OIDC issuer URL",
			envVars: map[string]string{
				EnvAuthMode:      "multi",
				EnvOIDCIssuerURL: "https://issuer.example.com",
			},
		},
		{
			name: "multi mode with OIDC client ID",
			envVars: map[string]string{
				EnvAuthMode:     "multi",
				EnvOIDCClientID: "my-client",
			},
		},
		{
			name: "multi mode with TLS enabled",
			envVars: map[string]string{
				EnvAuthMode:    "multi",
				EnvTLSEnabled:  "true",
				EnvTLSCertPath: "/path/to/cert.pem",
				EnvTLSKeyPath:  "/path/to/key.pem",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			if cfg.AuthMode != "multi" {
				t.Errorf("AuthMode = %s, want multi", cfg.AuthMode)
			}
		})
	}
}

func TestLoadVaultConfig(t *testing.T) {
	// Arrange
	clearEnvVars(t)
	t.Setenv(EnvVaultEnabled, "true")
	t.Setenv(EnvVaultAddr, "https://vault.example.com:8200")
	t.Setenv(EnvVaultToken, "s.mytoken123")
	t.Setenv(EnvVaultPKIPath, "pki/issue/my-role")
	t.Setenv(EnvVaultPKIRole, "my-role")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	if !cfg.VaultEnabled {
		t.Error("VaultEnabled should be true")
	}
	if cfg.VaultAddr != "https://vault.example.com:8200" {
		t.Errorf("VaultAddr = %s, want https://vault.example.com:8200", cfg.VaultAddr)
	}
	if cfg.VaultToken != "s.mytoken123" {
		t.Errorf("VaultToken = %s, want s.mytoken123", cfg.VaultToken)
	}
	if cfg.VaultPKIPath != "pki/issue/my-role" {
		t.Errorf("VaultPKIPath = %s, want pki/issue/my-role", cfg.VaultPKIPath)
	}
	if cfg.VaultPKIRole != "my-role" {
		t.Errorf("VaultPKIRole = %s, want my-role", cfg.VaultPKIRole)
	}
}

func TestLoadVaultConfigParseError(t *testing.T) {
	// Arrange
	clearEnvVars(t)
	t.Setenv(EnvVaultEnabled, "notabool")

	// Act
	cfg, err := Load()

	// Assert
	if err == nil {
		t.Fatal("Load() expected error, got nil")
	}
	if cfg != nil {
		t.Errorf("Load() expected nil config on error, got %+v", cfg)
	}
}

func TestBackwardCompatibility(t *testing.T) {
	// Arrange - no env vars set at all
	clearEnvVars(t)

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Verify all defaults work without any auth configuration
	if cfg.AuthMode != "none" {
		t.Errorf("AuthMode = %s, want none", cfg.AuthMode)
	}
	if cfg.TLSEnabled {
		t.Error("TLSEnabled should default to false")
	}
	if cfg.TLSCertPath != "" {
		t.Errorf("TLSCertPath should be empty, got %s", cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != "" {
		t.Errorf("TLSKeyPath should be empty, got %s", cfg.TLSKeyPath)
	}
	if cfg.TLSCAPath != "" {
		t.Errorf("TLSCAPath should be empty, got %s", cfg.TLSCAPath)
	}
	if cfg.OIDCIssuerURL != "" {
		t.Errorf("OIDCIssuerURL should be empty, got %s", cfg.OIDCIssuerURL)
	}
	if cfg.OIDCClientID != "" {
		t.Errorf("OIDCClientID should be empty, got %s", cfg.OIDCClientID)
	}
	if cfg.OIDCAudience != "" {
		t.Errorf("OIDCAudience should be empty, got %s", cfg.OIDCAudience)
	}
	if cfg.BasicAuthUsers != "" {
		t.Errorf("BasicAuthUsers should be empty, got %s", cfg.BasicAuthUsers)
	}
	if cfg.APIKeys != "" {
		t.Errorf("APIKeys should be empty, got %s", cfg.APIKeys)
	}
	if cfg.VaultEnabled {
		t.Error("VaultEnabled should default to false")
	}
	if cfg.VaultAddr != "" {
		t.Errorf("VaultAddr should be empty, got %s", cfg.VaultAddr)
	}
	if cfg.VaultToken != "" {
		t.Errorf("VaultToken should be empty, got %s", cfg.VaultToken)
	}
	if cfg.VaultPKIPath != "" {
		t.Errorf("VaultPKIPath should be empty, got %s", cfg.VaultPKIPath)
	}
	if cfg.VaultPKIRole != "" {
		t.Errorf("VaultPKIRole should be empty, got %s", cfg.VaultPKIRole)
	}
}

func TestConfig_ValidateAuth(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "valid config with auth mode none",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "none",
			},
			wantErr: nil,
		},
		{
			name: "valid config with auth mode mtls",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "mtls",
			},
			wantErr: nil,
		},
		{
			name: "valid config with auth mode oidc",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "oidc",
				OIDCIssuerURL:   "https://issuer.example.com",
				OIDCClientID:    "my-client",
			},
			wantErr: nil,
		},
		{
			name: "valid config with auth mode basic",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "basic",
				BasicAuthUsers:  "user1:hash1",
			},
			wantErr: nil,
		},
		{
			name: "valid config with auth mode apikey",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "apikey",
				APIKeys:         "key1:name1",
			},
			wantErr: nil,
		},
		{
			name: "invalid auth mode",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "invalid",
			},
			wantErr: ErrInvalidAuthMode,
		},
		{
			name: "oidc mode missing issuer",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "oidc",
				OIDCClientID:    "my-client",
			},
			wantErr: ErrInvalidOIDCConfig,
		},
		{
			name: "oidc mode missing client ID",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "oidc",
				OIDCIssuerURL:   "https://issuer.example.com",
			},
			wantErr: ErrInvalidOIDCConfig,
		},
		{
			name: "basic mode missing users",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "basic",
			},
			wantErr: ErrInvalidBasicAuthConfig,
		},
		{
			name: "apikey mode missing keys",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "apikey",
			},
			wantErr: ErrInvalidAPIKeyConfig,
		},
		{
			name: "multi mode without any auth config",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "multi",
			},
			wantErr: ErrInvalidMultiAuthConfig,
		},
		{
			name: "multi mode with basic auth",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "multi",
				BasicAuthUsers:  "user1:hash1",
			},
			wantErr: nil,
		},
		{
			name: "multi mode with API keys",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "multi",
				APIKeys:         "key1:name1",
			},
			wantErr: nil,
		},
		{
			name: "multi mode with TLS enabled",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "multi",
				TLSEnabled:      true,
				TLSCertPath:     "/path/to/cert.pem",
				TLSKeyPath:      "/path/to/key.pem",
			},
			wantErr: nil,
		},
		{
			name: "TLS enabled without cert",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				TLSEnabled:      true,
				TLSKeyPath:      "/path/to/key.pem",
			},
			wantErr: ErrInvalidTLSCertRequired,
		},
		{
			name: "TLS enabled without key",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				TLSEnabled:      true,
				TLSCertPath:     "/path/to/cert.pem",
			},
			wantErr: ErrInvalidTLSCertRequired,
		},
		{
			name: "TLS client auth require without CA",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				TLSEnabled:      true,
				TLSCertPath:     "/path/to/cert.pem",
				TLSKeyPath:      "/path/to/key.pem",
				TLSClientAuth:   "require",
			},
			wantErr: ErrInvalidTLSCARequired,
		},
		{
			name: "invalid TLS client auth",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				TLSClientAuth:   "invalid",
			},
			wantErr: ErrInvalidTLSClientAuth,
		},
		{
			name: "empty auth mode defaults to none",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				AuthMode:        "",
			},
			wantErr: nil,
		},
		{
			name: "empty TLS client auth defaults to none",
			config: Config{
				ServerPort:      8080,
				LogLevel:        "info",
				ShutdownTimeout: 30 * time.Second,
				TLSClientAuth:   "",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.config.Validate()

			// Assert
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error %v, got nil", tt.wantErr)
				} else if err != tt.wantErr {
					t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestLoadAuthEnvVariables(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *Config)
	}{
		{
			name: "OIDC environment variables loaded",
			envVars: map[string]string{
				EnvAuthMode:      "oidc",
				EnvOIDCIssuerURL: "https://auth.example.com",
				EnvOIDCClientID:  "client-123",
				EnvOIDCAudience:  "api://default",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.OIDCIssuerURL != "https://auth.example.com" {
					t.Errorf("OIDCIssuerURL = %s, want https://auth.example.com",
						cfg.OIDCIssuerURL)
				}
				if cfg.OIDCClientID != "client-123" {
					t.Errorf("OIDCClientID = %s, want client-123", cfg.OIDCClientID)
				}
				if cfg.OIDCAudience != "api://default" {
					t.Errorf("OIDCAudience = %s, want api://default", cfg.OIDCAudience)
				}
			},
		},
		{
			name: "basic auth environment variables loaded",
			envVars: map[string]string{
				EnvAuthMode:       "basic",
				EnvBasicAuthUsers: "admin:$2a$10$hash",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.BasicAuthUsers != "admin:$2a$10$hash" {
					t.Errorf("BasicAuthUsers = %s, want admin:$2a$10$hash",
						cfg.BasicAuthUsers)
				}
			},
		},
		{
			name: "API key environment variables loaded",
			envVars: map[string]string{
				EnvAuthMode: "apikey",
				EnvAPIKeys:  "test-key-123:test-service",
			},
			validate: func(t *testing.T, cfg *Config) {
				if cfg.APIKeys != "test-key-123:test-service" {
					t.Errorf("APIKeys = %s, want test-key-123:test-service",
						cfg.APIKeys)
				}
			},
		},
		{
			name: "TLS environment variables loaded",
			envVars: map[string]string{
				EnvTLSEnabled:    "true",
				EnvTLSCertPath:   "/etc/tls/cert.pem",
				EnvTLSKeyPath:    "/etc/tls/key.pem",
				EnvTLSCAPath:     "/etc/tls/ca.pem",
				EnvTLSClientAuth: "require",
			},
			validate: func(t *testing.T, cfg *Config) {
				if !cfg.TLSEnabled {
					t.Error("TLSEnabled should be true")
				}
				if cfg.TLSCertPath != "/etc/tls/cert.pem" {
					t.Errorf("TLSCertPath = %s, want /etc/tls/cert.pem",
						cfg.TLSCertPath)
				}
				if cfg.TLSKeyPath != "/etc/tls/key.pem" {
					t.Errorf("TLSKeyPath = %s, want /etc/tls/key.pem",
						cfg.TLSKeyPath)
				}
				if cfg.TLSCAPath != "/etc/tls/ca.pem" {
					t.Errorf("TLSCAPath = %s, want /etc/tls/ca.pem",
						cfg.TLSCAPath)
				}
				if cfg.TLSClientAuth != "require" {
					t.Errorf("TLSClientAuth = %s, want require",
						cfg.TLSClientAuth)
				}
			},
		},
		{
			name: "Vault environment variables loaded",
			envVars: map[string]string{
				EnvVaultEnabled: "true",
				EnvVaultAddr:    "https://vault:8200",
				EnvVaultToken:   "s.token",
				EnvVaultPKIPath: "pki/issue/role",
				EnvVaultPKIRole: "my-role",
			},
			validate: func(t *testing.T, cfg *Config) {
				if !cfg.VaultEnabled {
					t.Error("VaultEnabled should be true")
				}
				if cfg.VaultAddr != "https://vault:8200" {
					t.Errorf("VaultAddr = %s, want https://vault:8200",
						cfg.VaultAddr)
				}
				if cfg.VaultToken != "s.token" {
					t.Errorf("VaultToken = %s, want s.token", cfg.VaultToken)
				}
				if cfg.VaultPKIPath != "pki/issue/role" {
					t.Errorf("VaultPKIPath = %s, want pki/issue/role",
						cfg.VaultPKIPath)
				}
				if cfg.VaultPKIRole != "my-role" {
					t.Errorf("VaultPKIRole = %s, want my-role",
						cfg.VaultPKIRole)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Act
			cfg, err := Load()

			// Assert
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			tt.validate(t, cfg)
		})
	}
}

func TestHasAnyAuthConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{
			name:   "no auth config",
			config: Config{},
			want:   false,
		},
		{
			name:   "OIDC issuer URL set",
			config: Config{OIDCIssuerURL: "https://issuer.example.com"},
			want:   true,
		},
		{
			name:   "OIDC client ID set",
			config: Config{OIDCClientID: "my-client"},
			want:   true,
		},
		{
			name:   "basic auth users set",
			config: Config{BasicAuthUsers: "user1:hash1"},
			want:   true,
		},
		{
			name:   "API keys set",
			config: Config{APIKeys: "key1:name1"},
			want:   true,
		},
		{
			name:   "TLS enabled",
			config: Config{TLSEnabled: true},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := tt.config.hasAnyAuthConfig()

			// Assert
			if got != tt.want {
				t.Errorf("hasAnyAuthConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper functions

func clearEnvVars(t *testing.T) {
	t.Helper()
	envVars := []string{
		EnvServerPort,
		EnvLogLevel,
		EnvShutdownTimeout,
		EnvMetricsEnabled,
		EnvOTLPEndpoint,
		EnvAuthMode,
		EnvTLSEnabled,
		EnvTLSCertPath,
		EnvTLSKeyPath,
		EnvTLSCAPath,
		EnvTLSClientAuth,
		EnvOIDCIssuerURL,
		EnvOIDCClientID,
		EnvOIDCAudience,
		EnvBasicAuthUsers,
		EnvAPIKeys,
		EnvVaultEnabled,
		EnvVaultAddr,
		EnvVaultToken,
		EnvVaultPKIPath,
		EnvVaultPKIRole,
	}
	for _, env := range envVars {
		if err := os.Unsetenv(env); err != nil {
			t.Fatalf("failed to unset env var %s: %v", env, err)
		}
	}
}

func containsError(err, target error) bool {
	if err == nil {
		return target == nil
	}
	return err.Error() == target.Error() ||
		(len(err.Error()) > len(target.Error()) &&
			err.Error()[len(err.Error())-len(target.Error()):] == target.Error())
}
