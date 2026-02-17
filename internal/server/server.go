// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/handler"
	"github.com/vyrodovalexey/restapi-example/internal/middleware"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// Server represents the HTTP server.
type Server struct {
	httpServer    *http.Server
	router        *mux.Router
	config        *config.Config
	logger        *zap.Logger
	wsHandler     *handler.WebSocketHandler
	authenticator auth.Authenticator
	initErr       error // deferred error from initialization (e.g. TLS config)
}

// New creates a new Server instance.
// The authenticator parameter is optional; pass nil for no authentication.
// If TLS configuration fails, the error is deferred and returned by Start().
func New(
	cfg *config.Config,
	logger *zap.Logger,
	itemStore store.Store,
	authenticator auth.Authenticator,
) *Server {
	router := mux.NewRouter()

	s := &Server{
		router:        router,
		config:        cfg,
		logger:        logger,
		authenticator: authenticator,
	}

	s.setupMiddleware()
	s.setupRoutes(itemStore)
	s.initErr = s.setupHTTPServer()

	return s
}

// setupMiddleware configures the middleware chain.
func (s *Server) setupMiddleware() {
	allowedOrigins := []string{"*"}
	allowedMethods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodOptions,
	}
	allowedHeaders := []string{
		"Content-Type",
		"Authorization",
		middleware.RequestIDHeader,
		"X-API-Key",
	}

	// Apply middleware in order (first applied = outermost)
	s.router.Use(mux.MiddlewareFunc(middleware.Recovery(s.logger)))
	s.router.Use(mux.MiddlewareFunc(middleware.RequestID()))

	// Add metrics middleware if enabled
	if s.config.MetricsEnabled {
		s.router.Use(mux.MiddlewareFunc(middleware.Metrics()))
	}

	// Add auth middleware if authenticator is provided
	if s.authenticator != nil {
		s.router.Use(mux.MiddlewareFunc(
			middleware.Auth(s.authenticator, s.logger),
		))
	}

	s.router.Use(mux.MiddlewareFunc(middleware.Logging(s.logger)))
	s.router.Use(mux.MiddlewareFunc(
		middleware.CORS(allowedOrigins, allowedMethods, allowedHeaders),
	))
}

// setupRoutes configures the API routes.
func (s *Server) setupRoutes(itemStore store.Store) {
	// REST API handler
	restHandler := handler.NewRESTHandler(itemStore, s.logger)
	restHandler.RegisterRoutes(s.router)

	// WebSocket handler
	s.wsHandler = handler.NewWebSocketHandler(s.logger)
	s.wsHandler.RegisterRoutes(s.router)

	// Metrics endpoint
	if s.config.MetricsEnabled {
		s.router.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)
	}
}

// setupHTTPServer configures the HTTP server. It returns an error if TLS
// configuration is enabled but cannot be built.
func (s *Server) setupHTTPServer() error {
	s.httpServer = &http.Server{
		Addr:              s.config.Address(),
		Handler:           s.router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	if s.config.TLSEnabled {
		tlsConfig, err := s.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("building TLS config: %w", err)
		}
		s.httpServer.TLSConfig = tlsConfig
	}

	return nil
}

// buildTLSConfig creates a TLS configuration from the server config.
func (s *Server) buildTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(
		s.config.TLSCertPath, s.config.TLSKeyPath,
	)
	if err != nil {
		return nil, fmt.Errorf("loading TLS key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	switch s.config.TLSClientAuth {
	case "require":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	case "request":
		tlsConfig.ClientAuth = tls.RequestClientCert
	default:
		tlsConfig.ClientAuth = tls.NoClientCert
	}

	if s.config.TLSCAPath != "" {
		caCert, err := os.ReadFile(s.config.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("reading TLS CA cert: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("parsing TLS CA cert: no valid certificates found in %s", s.config.TLSCAPath)
		}
		tlsConfig.ClientCAs = caPool
	}

	return tlsConfig, nil
}

// Start starts the HTTP server. It returns any deferred initialization error
// (e.g. TLS configuration failure) before attempting to listen.
func (s *Server) Start() error {
	if s.initErr != nil {
		return fmt.Errorf("server initialization: %w", s.initErr)
	}

	if s.config.TLSEnabled {
		s.logger.Info("starting server with TLS",
			zap.String("address", s.config.Address()),
			zap.String("client_auth", s.config.TLSClientAuth),
		)
		err := s.httpServer.ListenAndServeTLS(
			s.config.TLSCertPath, s.config.TLSKeyPath,
		)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server listen and serve TLS: %w", err)
		}
	} else {
		s.logger.Info("starting server",
			zap.String("address", s.config.Address()),
		)
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server listen and serve: %w", err)
		}
	}

	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down server")

	// Close all WebSocket connections first
	if s.wsHandler != nil {
		s.wsHandler.CloseAllConnections()
	}

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	s.logger.Info("server shutdown complete")
	return nil
}

// Router returns the server's router for testing purposes.
func (s *Server) Router() *mux.Router {
	return s.router
}
