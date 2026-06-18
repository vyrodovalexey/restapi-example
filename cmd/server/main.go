// Package main is the entry point for the REST API server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/observability"
	"github.com/vyrodovalexey/restapi-example/internal/server"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// Build metadata injected at link time via -ldflags
// (e.g. -X main.Version=1.2.3). Defaults are used for local/dev builds.
var (
	// Version is the semantic version of the build.
	Version = "dev"
	// Commit is the VCS commit hash of the build.
	Commit = "unknown"
	// BuildTime is the UTC timestamp of the build.
	BuildTime = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Use a basic logger for startup errors
		basicLogger, _ := zap.NewProduction()
		basicLogger.Fatal("failed to load configuration", zap.Error(err))
	}

	// Initialize logger
	logger, err := initLogger(cfg.LogLevel)
	if err != nil {
		basicLogger, _ := zap.NewProduction()
		basicLogger.Fatal("failed to initialize logger", zap.Error(err))
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("configuration loaded",
		zap.Int("server_port", cfg.ServerPort),
		zap.Int("probe_port", cfg.ProbePort),
		zap.String("log_level", cfg.LogLevel),
		zap.Duration("shutdown_timeout", cfg.ShutdownTimeout),
		zap.Bool("metrics_enabled", cfg.MetricsEnabled),
		zap.String("auth_mode", cfg.AuthMode),
		zap.Bool("tls_enabled", cfg.TLSEnabled),
		zap.String("version", Version),
		zap.String("commit", Commit),
	)

	// Expose build metadata via the build_info Prometheus gauge.
	observability.SetBuildInfo(Version, Commit, BuildTime)

	// Root context for telemetry init and reuse across the lifecycle.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Initialize OpenTelemetry tracing. When APP_OTLP_ENDPOINT is unset this
	// installs a no-op tracer (zero network calls, never blocks startup).
	telemetry := observability.NewProvider(logger)
	initCtx, initCancel := context.WithTimeout(rootCtx, cfg.ShutdownTimeout)
	if err := telemetry.Init(initCtx, observability.TracingConfig{
		OTLPEndpoint:   cfg.OTLPEndpoint,
		ServiceVersion: Version,
	}); err != nil {
		// Telemetry must never prevent the service from starting.
		logger.Warn("failed to initialize tracing; continuing without OTLP export", zap.Error(err))
	}
	initCancel()

	// Create authenticator based on config
	authenticator, err := createAuthenticator(cfg, logger)
	if err != nil {
		logger.Fatal("failed to create authenticator", zap.Error(err))
	}

	// Create memory store wrapped with metrics instrumentation.
	itemStore := store.NewInstrumentedStore(store.NewMemoryStore())

	// Create and start server (pass authenticator + tracer)
	srv := server.New(cfg, logger, itemStore, authenticator, telemetry.Tracer())

	// Start server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- srv.Start()
	}()

	// Wait for shutdown signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("server error", zap.Error(err))
		return 1
	case sig := <-shutdown:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))

		// Create shutdown context with timeout (reuse the root context).
		ctx, cancel := context.WithTimeout(rootCtx, cfg.ShutdownTimeout)
		defer cancel()

		// Graceful shutdown
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", zap.Error(err))
			return 1
		}

		// Flush any buffered spans within the shutdown deadline.
		if err := telemetry.Shutdown(ctx); err != nil {
			logger.Error("telemetry shutdown failed", zap.Error(err))
		}
	}

	logger.Info("server stopped")
	return 0
}

// initLogger initializes a zap logger with the specified log level.
func initLogger(level string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	zapConfig := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapLevel),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding: "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "message",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	return zapConfig.Build()
}

// createAuthenticator creates an authenticator based on the config auth mode.
func createAuthenticator(
	cfg *config.Config,
	logger *zap.Logger,
) (auth.Authenticator, error) {
	switch cfg.AuthMode {
	case "none", "":
		logger.Info("authentication disabled")
		return nil, nil
	case "mtls":
		logger.Info("authentication mode: mTLS")
		return auth.NewMTLSAuthenticator(), nil
	case "basic":
		logger.Info("authentication mode: basic auth")
		return auth.NewBasicAuthenticator(cfg.BasicAuthUsers)
	case "apikey":
		logger.Info("authentication mode: API key")
		return auth.NewAPIKeyAuthenticator(cfg.APIKeys)
	case "oidc":
		logger.Info("authentication mode: OIDC",
			zap.String("issuer_url", cfg.OIDCIssuerURL),
			zap.String("client_id", cfg.OIDCClientID),
		)
		verifier, err := auth.NewOIDCTokenVerifier(cfg.OIDCIssuerURL)
		if err != nil {
			return nil, fmt.Errorf(
				"creating OIDC token verifier: %w", err,
			)
		}
		return auth.NewOIDCAuthenticator(
			verifier, cfg.OIDCAudience,
		), nil
	case "multi":
		logger.Info("authentication mode: multi")
		return createMultiAuthenticator(cfg, logger)
	default:
		return nil, fmt.Errorf("unknown auth mode: %s", cfg.AuthMode)
	}
}

// createMultiAuthenticator creates a multi-method authenticator
// from the available auth configurations.
func createMultiAuthenticator(
	cfg *config.Config,
	logger *zap.Logger,
) (auth.Authenticator, error) {
	var authenticators []auth.Authenticator

	if cfg.TLSEnabled && cfg.TLSClientAuth == "require" {
		authenticators = append(
			authenticators, auth.NewMTLSAuthenticator(),
		)
		logger.Info("multi-auth: mTLS enabled")
	}

	if cfg.BasicAuthUsers != "" {
		ba, err := auth.NewBasicAuthenticator(cfg.BasicAuthUsers)
		if err != nil {
			return nil, fmt.Errorf(
				"creating basic authenticator: %w", err,
			)
		}
		authenticators = append(authenticators, ba)
		logger.Info("multi-auth: basic auth enabled")
	}

	if cfg.APIKeys != "" {
		ak, err := auth.NewAPIKeyAuthenticator(cfg.APIKeys)
		if err != nil {
			return nil, fmt.Errorf(
				"creating API key authenticator: %w", err,
			)
		}
		authenticators = append(authenticators, ak)
		logger.Info("multi-auth: API key auth enabled")
	}

	if cfg.OIDCIssuerURL != "" && cfg.OIDCClientID != "" {
		verifier, err := auth.NewOIDCTokenVerifier(cfg.OIDCIssuerURL)
		if err != nil {
			return nil, fmt.Errorf(
				"creating OIDC token verifier for multi-auth: %w",
				err,
			)
		}
		authenticators = append(
			authenticators,
			auth.NewOIDCAuthenticator(verifier, cfg.OIDCAudience),
		)
		logger.Info("multi-auth: OIDC enabled")
	}

	if len(authenticators) == 0 {
		return nil, fmt.Errorf(
			"multi auth mode requires at least one authenticator",
		)
	}

	return auth.NewMultiAuthenticator(authenticators...), nil
}
