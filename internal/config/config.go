// Package config provides configuration management for the REST API server.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Default configuration values.
const (
	DefaultServerPort      = 8080
	DefaultLogLevel        = "info"
	DefaultShutdownTimeout = 30 * time.Second
	DefaultMetricsEnabled  = true
)

// Environment variable names.
const (
	EnvServerPort      = "APP_SERVER_PORT"
	EnvLogLevel        = "APP_LOG_LEVEL"
	EnvShutdownTimeout = "APP_SHUTDOWN_TIMEOUT"
	EnvMetricsEnabled  = "APP_METRICS_ENABLED"
	EnvOTLPEndpoint    = "APP_OTLP_ENDPOINT"
)

// Config holds the application configuration.
type Config struct {
	ServerPort      int
	LogLevel        string
	ShutdownTimeout time.Duration
	MetricsEnabled  bool
	OTLPEndpoint    string
}

// Validation errors.
var (
	ErrInvalidServerPort      = errors.New("server port must be between 1 and 65535")
	ErrInvalidLogLevel        = errors.New("log level must be one of: debug, info, warn, error")
	ErrInvalidShutdownTimeout = errors.New("shutdown timeout must be positive")
)

// Load reads configuration from environment variables with defaults.
// Environment variables have priority over default values.
func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:      DefaultServerPort,
		LogLevel:        DefaultLogLevel,
		ShutdownTimeout: DefaultShutdownTimeout,
		MetricsEnabled:  DefaultMetricsEnabled,
		OTLPEndpoint:    "",
	}

	if err := cfg.loadFromEnv(); err != nil {
		return nil, fmt.Errorf("loading config from environment: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// loadFromEnv loads configuration values from environment variables.
func (c *Config) loadFromEnv() error {
	if val := os.Getenv(EnvServerPort); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", EnvServerPort, err)
		}
		c.ServerPort = port
	}

	if val := os.Getenv(EnvLogLevel); val != "" {
		c.LogLevel = val
	}

	if val := os.Getenv(EnvShutdownTimeout); val != "" {
		timeout, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", EnvShutdownTimeout, err)
		}
		c.ShutdownTimeout = timeout
	}

	if val := os.Getenv(EnvMetricsEnabled); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", EnvMetricsEnabled, err)
		}
		c.MetricsEnabled = enabled
	}

	if val := os.Getenv(EnvOTLPEndpoint); val != "" {
		c.OTLPEndpoint = val
	}

	return nil
}

// Validate checks if the configuration values are valid.
func (c *Config) Validate() error {
	if c.ServerPort < 1 || c.ServerPort > 65535 {
		return ErrInvalidServerPort
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogLevel] {
		return ErrInvalidLogLevel
	}

	if c.ShutdownTimeout <= 0 {
		return ErrInvalidShutdownTimeout
	}

	return nil
}

// Address returns the server address in host:port format.
func (c *Config) Address() string {
	return fmt.Sprintf(":%d", c.ServerPort)
}
