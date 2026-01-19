// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/config"
	"github.com/vyrodovalexey/restapi-example/internal/handler"
	"github.com/vyrodovalexey/restapi-example/internal/middleware"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// Server represents the HTTP server.
type Server struct {
	httpServer *http.Server
	router     *mux.Router
	config     *config.Config
	logger     *zap.Logger
	wsHandler  *handler.WebSocketHandler
}

// New creates a new Server instance.
func New(cfg *config.Config, logger *zap.Logger, itemStore store.Store) *Server {
	router := mux.NewRouter()

	s := &Server{
		router: router,
		config: cfg,
		logger: logger,
	}

	s.setupMiddleware()
	s.setupRoutes(itemStore)
	s.setupHTTPServer()

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
	}

	// Apply middleware in order (first applied = outermost)
	s.router.Use(mux.MiddlewareFunc(middleware.Recovery(s.logger)))
	s.router.Use(mux.MiddlewareFunc(middleware.RequestID()))

	// Add metrics middleware if enabled
	if s.config.MetricsEnabled {
		s.router.Use(mux.MiddlewareFunc(middleware.Metrics()))
	}

	s.router.Use(mux.MiddlewareFunc(middleware.Logging(s.logger)))
	s.router.Use(mux.MiddlewareFunc(middleware.CORS(allowedOrigins, allowedMethods, allowedHeaders)))
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

// setupHTTPServer configures the HTTP server.
func (s *Server) setupHTTPServer() {
	s.httpServer = &http.Server{
		Addr:              s.config.Address(),
		Handler:           s.router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.logger.Info("starting server",
		zap.String("address", s.config.Address()),
		zap.Bool("metrics_enabled", s.config.MetricsEnabled),
	)

	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server listen and serve: %w", err)
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
