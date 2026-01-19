package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// Version is the application version.
const Version = "1.0.0"

// RESTHandler handles REST API requests for items.
type RESTHandler struct {
	store  store.Store
	logger *zap.Logger
}

// NewRESTHandler creates a new RESTHandler instance.
func NewRESTHandler(s store.Store, logger *zap.Logger) *RESTHandler {
	return &RESTHandler{
		store:  s,
		logger: logger,
	}
}

// RegisterRoutes registers the REST API routes with the router.
func (h *RESTHandler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/health", h.HealthCheck).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/items", h.ListItems).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/items", h.CreateItem).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/items/{id}", h.GetItem).Methods(http.MethodGet)
	router.HandleFunc("/api/v1/items/{id}", h.UpdateItem).Methods(http.MethodPut)
	router.HandleFunc("/api/v1/items/{id}", h.DeleteItem).Methods(http.MethodDelete)
}

// HealthCheck handles GET /health requests.
func (h *RESTHandler) HealthCheck(w http.ResponseWriter, _ *http.Request) {
	response := HealthResponse{
		Status:  "healthy",
		Version: Version,
	}
	h.writeJSON(w, http.StatusOK, model.NewSuccessResponse(response))
}

// ListItems handles GET /api/v1/items requests.
func (h *RESTHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items, err := h.store.List(ctx)
	if err != nil {
		h.logger.Error("failed to list items", zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "failed to retrieve items")
		return
	}

	h.writeJSON(w, http.StatusOK, model.NewSuccessResponse(items))
}

// GetItem handles GET /api/v1/items/{id} requests.
func (h *RESTHandler) GetItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	item, err := h.store.Get(ctx, id)
	if err != nil {
		h.handleStoreError(w, err, "get item")
		return
	}

	h.writeJSON(w, http.StatusOK, model.NewSuccessResponse(item))
}

// CreateItem handles POST /api/v1/items requests.
func (h *RESTHandler) CreateItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var input model.Item
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.logger.Warn("invalid request body", zap.Error(err))
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := input.Validate(); err != nil {
		h.logger.Warn("validation failed", zap.Error(err))
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	item, err := h.store.Create(ctx, &input)
	if err != nil {
		h.handleStoreError(w, err, "create item")
		return
	}

	h.writeJSON(w, http.StatusCreated, model.NewSuccessResponse(item))
}

// UpdateItem handles PUT /api/v1/items/{id} requests.
func (h *RESTHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	var input model.Item
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.logger.Warn("invalid request body", zap.Error(err))
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := input.Validate(); err != nil {
		h.logger.Warn("validation failed", zap.Error(err))
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	item, err := h.store.Update(ctx, id, &input)
	if err != nil {
		h.handleStoreError(w, err, "update item")
		return
	}

	h.writeJSON(w, http.StatusOK, model.NewSuccessResponse(item))
}

// DeleteItem handles DELETE /api/v1/items/{id} requests.
func (h *RESTHandler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.store.Delete(ctx, id); err != nil {
		h.handleStoreError(w, err, "delete item")
		return
	}

	h.writeJSON(w, http.StatusNoContent, nil)
}

// handleStoreError handles store errors and writes appropriate HTTP responses.
func (h *RESTHandler) handleStoreError(w http.ResponseWriter, err error, operation string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "item not found")
	case errors.Is(err, store.ErrInvalidID):
		h.writeError(w, http.StatusBadRequest, "invalid item ID")
	case errors.Is(err, store.ErrAlreadyExists):
		h.writeError(w, http.StatusConflict, "item already exists")
	default:
		h.logger.Error("store operation failed", zap.String("operation", operation), zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// writeJSON writes a JSON response with the given status code.
func (h *RESTHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if data == nil {
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", zap.Error(err))
	}
}

// writeError writes an error response with the given status code and message.
func (h *RESTHandler) writeError(w http.ResponseWriter, status int, message string) {
	response := model.ErrorResponse{
		Code:    status,
		Message: message,
	}
	h.writeJSON(w, status, response)
}
