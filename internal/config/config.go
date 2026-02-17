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
	DefaultAuthMode        = "none"
	DefaultTLSClientAuth   = "none"
	DefaultProbePort       = 9090
)

// Environment variable names.
const (
	EnvServerPort      = "APP_SERVER_PORT"
	EnvLogLevel        = "APP_LOG_LEVEL"
	EnvShutdownTimeout = "APP_SHUTDOWN_TIMEOUT"
	EnvMetricsEnabled  = "APP_METRICS_ENABLED"
	EnvOTLPEndpoint    = "APP_OTLP_ENDPOINT"
	EnvAuthMode        = "APP_AUTH_MODE"
	EnvTLSEnabled      = "APP_TLS_ENABLED"
	EnvTLSCertPath     = "APP_TLS_CERT_PATH"
	EnvTLSKeyPath      = "APP_TLS_KEY_PATH"
	EnvTLSCAPath       = "APP_TLS_CA_PATH"
	EnvTLSClientAuth   = "APP_TLS_CLIENT_AUTH"
	EnvOIDCIssuerURL   = "APP_OIDC_ISSUER_URL"
	EnvOIDCClientID    = "APP_OIDC_CLIENT_ID"
	EnvOIDCAudience    = "APP_OIDC_AUDIENCE"
	EnvBasicAuthUsers  = "APP_BASIC_AUTH_USERS"
	EnvAPIKeys         = "APP_API_KEYS" //nolint:gosec // env var name, not a credential
	EnvVaultEnabled    = "APP_VAULT_ENABLED"
	EnvVaultAddr       = "APP_VAULT_ADDR"
	EnvVaultToken      = "APP_VAULT_TOKEN" //nolint:gosec // env var name, not a credential
	EnvVaultPKIPath    = "APP_VAULT_PKI_PATH"
	EnvVaultPKIRole    = "APP_VAULT_PKI_ROLE"
	EnvProbePort       = "APP_PROBE_PORT"
)

// Config holds the application configuration.
type Config struct {
	// Server settings.
	ServerPort      int
	ProbePort       int // Probe server port (0 = disabled).
	LogLevel        string
	ShutdownTimeout time.Duration
	MetricsEnabled  bool
	OTLPEndpoint    string

	// Authentication mode: none, mtls, oidc, basic, apikey, multi.
	AuthMode string

	// TLS settings.
	TLSEnabled    bool
	TLSCertPath   string
	TLSKeyPath    string
	TLSCAPath     string
	TLSClientAuth string

	// OIDC settings.
	OIDCIssuerURL string
	OIDCClientID  string
	OIDCAudience  string

	// Basic auth settings (format: "user1:bcrypt_hash,user2:bcrypt_hash").
	BasicAuthUsers string

	// API key settings (format: "key1:name1,key2:name2").
	APIKeys string

	// Vault settings.
	VaultEnabled bool
	VaultAddr    string
	VaultToken   string
	VaultPKIPath string
	VaultPKIRole string
}

// Validation errors.
var (
	ErrInvalidServerPort      = errors.New("server port must be between 1 and 65535")
	ErrInvalidLogLevel        = errors.New("log level must be one of: debug, info, warn, error")
	ErrInvalidShutdownTimeout = errors.New("shutdown timeout must be positive")
	ErrInvalidAuthMode        = errors.New(
		"auth mode must be one of: none, mtls, oidc, basic, apikey, multi",
	)
	ErrInvalidTLSClientAuth = errors.New(
		"TLS client auth must be one of: none, request, require",
	)
	ErrInvalidTLSCertRequired = errors.New(
		"TLS cert path and key path must be set when TLS is enabled",
	)
	ErrInvalidTLSCARequired = errors.New(
		"TLS CA path must be set when TLS client auth is require",
	)
	ErrInvalidOIDCConfig = errors.New(
		"OIDC issuer URL and client ID must be set when auth mode is oidc",
	)
	ErrInvalidBasicAuthConfig = errors.New(
		"basic auth users must be set when auth mode is basic",
	)
	ErrInvalidAPIKeyConfig = errors.New(
		"API keys must be set when auth mode is apikey",
	)
	ErrInvalidMultiAuthConfig = errors.New(
		"at least one auth config must be provided when auth mode is multi",
	)
	ErrInvalidProbePort = errors.New(
		"probe port must be between 0 and 65535",
	)
	ErrProbePortConflict = errors.New(
		"probe port must differ from server port when probe port is not 0",
	)
)

// Load reads configuration from environment variables with defaults.
// Environment variables have priority over default values.
func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:      DefaultServerPort,
		ProbePort:       DefaultProbePort,
		LogLevel:        DefaultLogLevel,
		ShutdownTimeout: DefaultShutdownTimeout,
		MetricsEnabled:  DefaultMetricsEnabled,
		OTLPEndpoint:    "",
		AuthMode:        DefaultAuthMode,
		TLSClientAuth:   DefaultTLSClientAuth,
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
	if err := c.loadServerEnv(); err != nil {
		return err
	}

	if err := c.loadAuthEnv(); err != nil {
		return err
	}

	return nil
}

// loadServerEnv loads server-related environment variables.
func (c *Config) loadServerEnv() error {
	if val := os.Getenv(EnvServerPort); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", EnvServerPort, err)
		}
		c.ServerPort = port
	}

	if val := os.Getenv(EnvProbePort); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", EnvProbePort, err)
		}
		c.ProbePort = port
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

// loadAuthEnv loads authentication and security environment variables.
func (c *Config) loadAuthEnv() error {
	if val := os.Getenv(EnvAuthMode); val != "" {
		c.AuthMode = val
	}

	if err := c.loadTLSEnv(); err != nil {
		return err
	}

	c.loadOIDCEnv()
	c.loadBasicAuthEnv()
	c.loadAPIKeyEnv()

	if err := c.loadVaultEnv(); err != nil {
		return err
	}

	return nil
}

// loadTLSEnv loads TLS-related environment variables.
func (c *Config) loadTLSEnv() error {
	if val := os.Getenv(EnvTLSEnabled); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", EnvTLSEnabled, err)
		}
		c.TLSEnabled = enabled
	}

	if val := os.Getenv(EnvTLSCertPath); val != "" {
		c.TLSCertPath = val
	}

	if val := os.Getenv(EnvTLSKeyPath); val != "" {
		c.TLSKeyPath = val
	}

	if val := os.Getenv(EnvTLSCAPath); val != "" {
		c.TLSCAPath = val
	}

	if val := os.Getenv(EnvTLSClientAuth); val != "" {
		c.TLSClientAuth = val
	}

	return nil
}

// loadOIDCEnv loads OIDC-related environment variables.
func (c *Config) loadOIDCEnv() {
	if val := os.Getenv(EnvOIDCIssuerURL); val != "" {
		c.OIDCIssuerURL = val
	}

	if val := os.Getenv(EnvOIDCClientID); val != "" {
		c.OIDCClientID = val
	}

	if val := os.Getenv(EnvOIDCAudience); val != "" {
		c.OIDCAudience = val
	}
}

// loadBasicAuthEnv loads basic auth environment variables.
func (c *Config) loadBasicAuthEnv() {
	if val := os.Getenv(EnvBasicAuthUsers); val != "" {
		c.BasicAuthUsers = val
	}
}

// loadAPIKeyEnv loads API key environment variables.
func (c *Config) loadAPIKeyEnv() {
	if val := os.Getenv(EnvAPIKeys); val != "" {
		c.APIKeys = val
	}
}

// loadVaultEnv loads Vault-related environment variables.
func (c *Config) loadVaultEnv() error {
	if val := os.Getenv(EnvVaultEnabled); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", EnvVaultEnabled, err)
		}
		c.VaultEnabled = enabled
	}

	if val := os.Getenv(EnvVaultAddr); val != "" {
		c.VaultAddr = val
	}

	if val := os.Getenv(EnvVaultToken); val != "" {
		c.VaultToken = val
	}

	if val := os.Getenv(EnvVaultPKIPath); val != "" {
		c.VaultPKIPath = val
	}

	if val := os.Getenv(EnvVaultPKIRole); val != "" {
		c.VaultPKIRole = val
	}

	return nil
}

// Validate checks if the configuration values are valid.
func (c *Config) Validate() error {
	if err := c.validateServer(); err != nil {
		return err
	}

	if err := c.validateAuth(); err != nil {
		return err
	}

	return nil
}

// validateServer validates server-related configuration.
func (c *Config) validateServer() error {
	if c.ServerPort < 1 || c.ServerPort > 65535 {
		return ErrInvalidServerPort
	}

	if c.ProbePort != 0 && (c.ProbePort < 1 || c.ProbePort > 65535) {
		return ErrInvalidProbePort
	}

	if c.ProbePort != 0 && c.ProbePort == c.ServerPort {
		return ErrProbePortConflict
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

// validateAuth validates authentication and security configuration.
func (c *Config) validateAuth() error {
	authMode := c.authModeOrDefault()

	if err := c.validateAuthMode(authMode); err != nil {
		return err
	}

	if err := c.validateTLS(); err != nil {
		return err
	}

	if err := c.validateAuthModeRequirements(authMode); err != nil {
		return err
	}

	return nil
}

// authModeOrDefault returns the auth mode, defaulting to "none" if empty.
func (c *Config) authModeOrDefault() string {
	if c.AuthMode == "" {
		return DefaultAuthMode
	}
	return c.AuthMode
}

// tlsClientAuthOrDefault returns the TLS client auth, defaulting to "none" if empty.
func (c *Config) tlsClientAuthOrDefault() string {
	if c.TLSClientAuth == "" {
		return DefaultTLSClientAuth
	}
	return c.TLSClientAuth
}

// validateAuthMode checks that the auth mode is a valid value.
func (c *Config) validateAuthMode(authMode string) error {
	validAuthModes := map[string]bool{
		"none":   true,
		"mtls":   true,
		"oidc":   true,
		"basic":  true,
		"apikey": true,
		"multi":  true,
	}
	if !validAuthModes[authMode] {
		return ErrInvalidAuthMode
	}

	return nil
}

// validateTLS validates TLS-related configuration.
func (c *Config) validateTLS() error {
	clientAuth := c.tlsClientAuthOrDefault()

	validClientAuth := map[string]bool{
		"none":    true,
		"request": true,
		"require": true,
	}
	if !validClientAuth[clientAuth] {
		return ErrInvalidTLSClientAuth
	}

	if c.TLSEnabled && (c.TLSCertPath == "" || c.TLSKeyPath == "") {
		return ErrInvalidTLSCertRequired
	}

	if clientAuth == "require" && c.TLSCAPath == "" {
		return ErrInvalidTLSCARequired
	}

	return nil
}

// validateAuthModeRequirements validates auth-mode-specific requirements.
func (c *Config) validateAuthModeRequirements(authMode string) error {
	switch authMode {
	case "oidc":
		if c.OIDCIssuerURL == "" || c.OIDCClientID == "" {
			return ErrInvalidOIDCConfig
		}
	case "basic":
		if c.BasicAuthUsers == "" {
			return ErrInvalidBasicAuthConfig
		}
	case "apikey":
		if c.APIKeys == "" {
			return ErrInvalidAPIKeyConfig
		}
	case "multi":
		if !c.hasAnyAuthConfig() {
			return ErrInvalidMultiAuthConfig
		}
	}

	return nil
}

// hasAnyAuthConfig checks if at least one auth-related configuration is provided.
func (c *Config) hasAnyAuthConfig() bool {
	return c.OIDCIssuerURL != "" ||
		c.OIDCClientID != "" ||
		c.BasicAuthUsers != "" ||
		c.APIKeys != "" ||
		c.TLSEnabled
}

// Address returns the server address in host:port format.
func (c *Config) Address() string {
	return fmt.Sprintf(":%d", c.ServerPort)
}

// ProbeAddress returns the probe server address in host:port format.
func (c *Config) ProbeAddress() string {
	return fmt.Sprintf(":%d", c.ProbePort)
}
