package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// mockStore implements store.Store for testing
type mockStore struct {
	items      map[string]model.Item
	listErr    error
	getErr     error
	createErr  error
	updateErr  error
	deleteErr  error
	createItem *model.Item
	updateItem *model.Item
}

func newMockStore() *mockStore {
	return &mockStore{
		items: make(map[string]model.Item),
	}
}

func (m *mockStore) List(_ context.Context) ([]model.Item, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	items := make([]model.Item, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	return items, nil
}

func (m *mockStore) Get(_ context.Context, id string) (*model.Item, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	item, exists := m.items[id]
	if !exists {
		return nil, store.ErrNotFound
	}
	return &item, nil
}

func (m *mockStore) Create(_ context.Context, item *model.Item) (*model.Item, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.createItem != nil {
		return m.createItem, nil
	}
	newItem := *item
	newItem.ID = "generated-id"
	m.items[newItem.ID] = newItem
	return &newItem, nil
}

func (m *mockStore) Update(_ context.Context, id string, item *model.Item) (*model.Item, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	if m.updateItem != nil {
		return m.updateItem, nil
	}
	if _, exists := m.items[id]; !exists {
		return nil, store.ErrNotFound
	}
	updatedItem := *item
	updatedItem.ID = id
	m.items[id] = updatedItem
	return &updatedItem, nil
}

func (m *mockStore) Delete(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	if _, exists := m.items[id]; !exists {
		return store.ErrNotFound
	}
	delete(m.items, id)
	return nil
}

func TestNewRESTHandler(t *testing.T) {
	// Arrange
	mockStore := newMockStore()
	logger := zap.NewNop()

	// Act
	handler := NewRESTHandler(mockStore, logger)

	// Assert
	if handler == nil {
		t.Fatal("NewRESTHandler() returned nil")
	}
	if handler.store == nil {
		t.Error("store should not be nil")
	}
	if handler.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestRESTHandler_HealthCheck(t *testing.T) {
	// Arrange
	mockStore := newMockStore()
	logger := zap.NewNop()
	handler := NewRESTHandler(mockStore, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.HealthCheck(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("HealthCheck() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var response model.APIResponse[HealthResponse]
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.Success {
		t.Error("HealthCheck() response.Success = false, want true")
	}
	if response.Data.Status != "healthy" {
		t.Errorf("HealthCheck() status = %s, want healthy", response.Data.Status)
	}
	if response.Data.Version != Version {
		t.Errorf("HealthCheck() version = %s, want %s", response.Data.Version, Version)
	}
}

func TestRESTHandler_ListItems(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*mockStore)
		wantStatus int
		wantCount  int
		wantErr    bool
	}{
		{
			name: "empty list",
			setup: func(_ *mockStore) {
				// No items
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
			wantErr:    false,
		},
		{
			name: "single item",
			setup: func(m *mockStore) {
				m.items["1"] = model.Item{ID: "1", Name: "Item 1", Price: 10}
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
			wantErr:    false,
		},
		{
			name: "multiple items",
			setup: func(m *mockStore) {
				m.items["1"] = model.Item{ID: "1", Name: "Item 1", Price: 10}
				m.items["2"] = model.Item{ID: "2", Name: "Item 2", Price: 20}
				m.items["3"] = model.Item{ID: "3", Name: "Item 3", Price: 30}
			},
			wantStatus: http.StatusOK,
			wantCount:  3,
			wantErr:    false,
		},
		{
			name: "store error",
			setup: func(m *mockStore) {
				m.listErr = errors.New("database error")
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStore := newMockStore()
			tt.setup(mockStore)
			logger := zap.NewNop()
			handler := NewRESTHandler(mockStore, logger)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
			rr := httptest.NewRecorder()

			// Act
			handler.ListItems(rr, req)

			// Assert
			if rr.Code != tt.wantStatus {
				t.Errorf("ListItems() status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if !tt.wantErr {
				var response model.APIResponse[[]model.Item]
				if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if !response.Success {
					t.Error("ListItems() response.Success = false, want true")
				}
				if len(response.Data) != tt.wantCount {
					t.Errorf("ListItems() count = %d, want %d", len(response.Data), tt.wantCount)
				}
			}
		})
	}
}

func TestRESTHandler_GetItem(t *testing.T) {
	tests := []struct {
		name       string
		itemID     string
		setup      func(*mockStore)
		wantStatus int
		wantErr    bool
	}{
		{
			name:   "existing item",
			itemID: "123",
			setup: func(m *mockStore) {
				m.items["123"] = model.Item{ID: "123", Name: "Test Item", Price: 9.99}
			},
			wantStatus: http.StatusOK,
			wantErr:    false,
		},
		{
			name:   "non-existing item",
			itemID: "non-existent",
			setup: func(_ *mockStore) {
				// No items
			},
			wantStatus: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:   "invalid id",
			itemID: "invalid",
			setup: func(m *mockStore) {
				m.getErr = store.ErrInvalidID
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:   "store error",
			itemID: "123",
			setup: func(m *mockStore) {
				m.getErr = errors.New("database error")
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStore := newMockStore()
			tt.setup(mockStore)
			logger := zap.NewNop()
			handler := NewRESTHandler(mockStore, logger)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/items/"+tt.itemID, nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.itemID})
			rr := httptest.NewRecorder()

			// Act
			handler.GetItem(rr, req)

			// Assert
			if rr.Code != tt.wantStatus {
				t.Errorf("GetItem() status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if !tt.wantErr {
				var response model.APIResponse[*model.Item]
				if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if !response.Success {
					t.Error("GetItem() response.Success = false, want true")
				}
				if response.Data.ID != tt.itemID {
					t.Errorf("GetItem() ID = %s, want %s", response.Data.ID, tt.itemID)
				}
			}
		})
	}
}

func TestRESTHandler_CreateItem(t *testing.T) {
	tests := []struct {
		name       string
		body       interface{}
		setup      func(*mockStore)
		wantStatus int
		wantErr    bool
	}{
		{
			name: "valid item",
			body: model.Item{Name: "New Item", Description: "A new item", Price: 19.99},
			setup: func(m *mockStore) {
				m.createItem = &model.Item{ID: "new-id", Name: "New Item", Description: "A new item", Price: 19.99}
			},
			wantStatus: http.StatusCreated,
			wantErr:    false,
		},
		{
			name:       "invalid JSON",
			body:       "invalid json",
			setup:      func(_ *mockStore) {},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "empty name",
			body:       model.Item{Name: "", Price: 10},
			setup:      func(_ *mockStore) {},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "negative price",
			body:       model.Item{Name: "Test", Price: -10},
			setup:      func(_ *mockStore) {},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name: "store error",
			body: model.Item{Name: "Test Item", Price: 10},
			setup: func(m *mockStore) {
				m.createErr = errors.New("database error")
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name: "already exists",
			body: model.Item{Name: "Test Item", Price: 10},
			setup: func(m *mockStore) {
				m.createErr = store.ErrAlreadyExists
			},
			wantStatus: http.StatusConflict,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStore := newMockStore()
			tt.setup(mockStore)
			logger := zap.NewNop()
			handler := NewRESTHandler(mockStore, logger)

			var body []byte
			var err error
			if str, ok := tt.body.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("Failed to marshal body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/items", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// Act
			handler.CreateItem(rr, req)

			// Assert
			if rr.Code != tt.wantStatus {
				t.Errorf("CreateItem() status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if !tt.wantErr {
				var response model.APIResponse[*model.Item]
				if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if !response.Success {
					t.Error("CreateItem() response.Success = false, want true")
				}
				if response.Data.ID == "" {
					t.Error("CreateItem() should return item with ID")
				}
			}
		})
	}
}

func TestRESTHandler_UpdateItem(t *testing.T) {
	tests := []struct {
		name       string
		itemID     string
		body       interface{}
		setup      func(*mockStore)
		wantStatus int
		wantErr    bool
	}{
		{
			name:   "valid update",
			itemID: "123",
			body:   model.Item{Name: "Updated Item", Description: "Updated description", Price: 29.99},
			setup: func(m *mockStore) {
				m.items["123"] = model.Item{ID: "123", Name: "Original", Price: 10}
				m.updateItem = &model.Item{ID: "123", Name: "Updated Item", Description: "Updated description", Price: 29.99}
			},
			wantStatus: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "invalid JSON",
			itemID:     "123",
			body:       "invalid json",
			setup:      func(_ *mockStore) {},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "empty name",
			itemID:     "123",
			body:       model.Item{Name: "", Price: 10},
			setup:      func(_ *mockStore) {},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "negative price",
			itemID:     "123",
			body:       model.Item{Name: "Test", Price: -10},
			setup:      func(_ *mockStore) {},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:   "non-existing item",
			itemID: "non-existent",
			body:   model.Item{Name: "Test", Price: 10},
			setup: func(m *mockStore) {
				m.updateErr = store.ErrNotFound
			},
			wantStatus: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:   "invalid id",
			itemID: "invalid",
			body:   model.Item{Name: "Test", Price: 10},
			setup: func(m *mockStore) {
				m.updateErr = store.ErrInvalidID
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:   "store error",
			itemID: "123",
			body:   model.Item{Name: "Test", Price: 10},
			setup: func(m *mockStore) {
				m.updateErr = errors.New("database error")
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStore := newMockStore()
			tt.setup(mockStore)
			logger := zap.NewNop()
			handler := NewRESTHandler(mockStore, logger)

			var body []byte
			var err error
			if str, ok := tt.body.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("Failed to marshal body: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPut, "/api/v1/items/"+tt.itemID, bytes.NewReader(body))
			req = mux.SetURLVars(req, map[string]string{"id": tt.itemID})
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			// Act
			handler.UpdateItem(rr, req)

			// Assert
			if rr.Code != tt.wantStatus {
				t.Errorf("UpdateItem() status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if !tt.wantErr {
				var response model.APIResponse[*model.Item]
				if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				if !response.Success {
					t.Error("UpdateItem() response.Success = false, want true")
				}
			}
		})
	}
}

func TestRESTHandler_DeleteItem(t *testing.T) {
	tests := []struct {
		name       string
		itemID     string
		setup      func(*mockStore)
		wantStatus int
		wantErr    bool
	}{
		{
			name:   "existing item",
			itemID: "123",
			setup: func(m *mockStore) {
				m.items["123"] = model.Item{ID: "123", Name: "Test", Price: 10}
			},
			wantStatus: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:   "non-existing item",
			itemID: "non-existent",
			setup: func(m *mockStore) {
				m.deleteErr = store.ErrNotFound
			},
			wantStatus: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:   "invalid id",
			itemID: "invalid",
			setup: func(m *mockStore) {
				m.deleteErr = store.ErrInvalidID
			},
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:   "store error",
			itemID: "123",
			setup: func(m *mockStore) {
				m.deleteErr = errors.New("database error")
			},
			wantStatus: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			mockStore := newMockStore()
			tt.setup(mockStore)
			logger := zap.NewNop()
			handler := NewRESTHandler(mockStore, logger)

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/items/"+tt.itemID, nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.itemID})
			rr := httptest.NewRecorder()

			// Act
			handler.DeleteItem(rr, req)

			// Assert
			if rr.Code != tt.wantStatus {
				t.Errorf("DeleteItem() status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestRESTHandler_RegisterRoutes(t *testing.T) {
	// Arrange
	mockStore := newMockStore()
	mockStore.items["123"] = model.Item{ID: "123", Name: "Test", Price: 10}
	logger := zap.NewNop()
	handler := NewRESTHandler(mockStore, logger)
	router := mux.NewRouter()

	// Act
	handler.RegisterRoutes(router)

	// Assert - Test that routes are registered by making requests
	tests := []struct {
		method     string
		path       string
		wantStatus int
	}{
		{http.MethodGet, "/health", http.StatusOK},
		{http.MethodGet, "/api/v1/items", http.StatusOK},
		{http.MethodPost, "/api/v1/items", http.StatusCreated},
		{http.MethodGet, "/api/v1/items/123", http.StatusOK},
		{http.MethodPut, "/api/v1/items/123", http.StatusOK},
		{http.MethodDelete, "/api/v1/items/123", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			// Reset store for each test
			mockStore := newMockStore()
			mockStore.items["123"] = model.Item{ID: "123", Name: "Test", Price: 10}
			handler := NewRESTHandler(mockStore, logger)
			router := mux.NewRouter()
			handler.RegisterRoutes(router)

			var body *bytes.Reader
			if tt.method == http.MethodPost || tt.method == http.MethodPut {
				body = bytes.NewReader([]byte(`{"name":"Test","price":10}`))
			} else {
				body = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// Route should return expected status
			if rr.Code != tt.wantStatus {
				t.Errorf("Route %s %s status = %d, want %d", tt.method, tt.path, rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestRESTHandler_ContentType(t *testing.T) {
	// Arrange
	mockStore := newMockStore()
	logger := zap.NewNop()
	handler := NewRESTHandler(mockStore, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	// Act
	handler.HealthCheck(rr, req)

	// Assert
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", Version)
	}
}
