// Package handler provides HTTP request handlers for the REST API.
package handler

import (
	"net/http"

	"github.com/gorilla/mux"
)

// Handler defines the interface for HTTP handlers.
type Handler interface {
	// RegisterRoutes registers the handler's routes with the router.
	RegisterRoutes(router *mux.Router)
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// ReadyResponse represents the readiness check response.
type ReadyResponse struct {
	Status string `json:"status"`
}

// WriteJSON is a helper interface for handlers that write JSON responses.
type JSONWriter interface {
	WriteJSON(w http.ResponseWriter, status int, data any)
	WriteError(w http.ResponseWriter, status int, message string)
}
