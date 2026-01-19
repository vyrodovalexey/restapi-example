// Package main is the entry point for the REST API server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/server"
	"github.com/vyrodovalexey/restapi-example/internal/store"
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
		zap.String("log_level", cfg.LogLevel),
		zap.Duration("shutdown_timeout", cfg.ShutdownTimeout),
		zap.Bool("metrics_enabled", cfg.MetricsEnabled),
	)

	// Create memory store
	itemStore := store.NewMemoryStore()

	// Create and start server
	srv := server.New(cfg, logger, itemStore)

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

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		// Graceful shutdown
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", zap.Error(err))
			return 1
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
