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

// Helper functions

func clearEnvVars(t *testing.T) {
	t.Helper()
	envVars := []string{
		EnvServerPort,
		EnvLogLevel,
		EnvShutdownTimeout,
		EnvMetricsEnabled,
		EnvOTLPEndpoint,
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
