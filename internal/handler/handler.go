// Package handler provides HTTP request handlers for the REST API.
package handler

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// ReadyResponse represents the readiness check response.
type ReadyResponse struct {
	Status string `json:"status"`
}
