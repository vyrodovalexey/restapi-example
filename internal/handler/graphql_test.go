package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/model"
	"github.com/vyrodovalexey/restapi-example/internal/store"
)

// graphqlResponse is a helper struct for parsing GraphQL JSON responses.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphqlRequest creates an HTTP POST request with a GraphQL query body.
func graphqlRequest(query string) *http.Request {
	body := fmt.Sprintf(`{"query": %s}`, strconv.Quote(query))
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// setupGraphQLRouter creates a mux.Router with GraphQL routes registered.
func setupGraphQLRouter(ms *mockStore) *mux.Router {
	logger := zap.NewNop()
	h := NewGraphQLHandler(ms, logger)
	router := mux.NewRouter()
	h.RegisterRoutes(router)
	return router
}

// --- Constructor Tests ---

func TestNewGraphQLHandler(t *testing.T) {
	// Arrange
	ms := newMockStore()
	logger := zap.NewNop()

	// Act
	h := NewGraphQLHandler(ms, logger)

	// Assert
	if h == nil {
		t.Fatal("NewGraphQLHandler() returned nil")
	}
	if h.store == nil {
		t.Error("store should not be nil")
	}
	if h.logger == nil {
		t.Error("logger should not be nil")
	}
	if h.handler == nil {
		t.Error("handler should not be nil")
	}
}

func TestGraphQLHandler_RegisterRoutes(t *testing.T) {
	// Arrange
	ms := newMockStore()
	logger := zap.NewNop()
	h := NewGraphQLHandler(ms, logger)
	router := mux.NewRouter()

	// Act
	h.RegisterRoutes(router)

	// Assert - verify the /graphql route is registered by making a request
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// GET /graphql should return 200 with GraphiQL HTML (not 404/405)
	if rr.Code == http.StatusNotFound || rr.Code == http.StatusMethodNotAllowed {
		t.Errorf("RegisterRoutes() GET /graphql returned %d, expected route to be registered", rr.Code)
	}
}

// --- Query Tests ---

func TestGraphQLHandler_QueryItems_EmptyStore(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	query := `{ items { id name description price createdAt updatedAt } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItems() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("QueryItems() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if len(data.Items) != 0 {
		t.Errorf("QueryItems() returned %d items, want 0", len(data.Items))
	}
}

func TestGraphQLHandler_QueryItems_WithItems(t *testing.T) {
	// Arrange
	ms := newMockStore()
	now := time.Now().UTC()
	ms.items["1"] = model.Item{ID: "1", Name: "Item 1", Description: "Desc 1", Price: 10.5, CreatedAt: now, UpdatedAt: now}
	ms.items["2"] = model.Item{ID: "2", Name: "Item 2", Description: "Desc 2", Price: 20.0, CreatedAt: now, UpdatedAt: now}
	router := setupGraphQLRouter(ms)

	query := `{ items { id name description price createdAt updatedAt } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItems() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("QueryItems() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		Items []struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
			CreatedAt   string  `json:"createdAt"`
			UpdatedAt   string  `json:"updatedAt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if len(data.Items) != 2 {
		t.Errorf("QueryItems() returned %d items, want 2", len(data.Items))
	}

	// Verify items have expected fields populated
	for _, item := range data.Items {
		if item.ID == "" {
			t.Error("QueryItems() item ID should not be empty")
		}
		if item.Name == "" {
			t.Error("QueryItems() item Name should not be empty")
		}
		if item.CreatedAt == "" {
			t.Error("QueryItems() item CreatedAt should not be empty")
		}
		if item.UpdatedAt == "" {
			t.Error("QueryItems() item UpdatedAt should not be empty")
		}
	}
}

func TestGraphQLHandler_QueryItems_StoreError(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.listErr = errors.New("database connection failed")
	router := setupGraphQLRouter(ms)

	query := `{ items { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItems() status = %d, want %d (GraphQL errors are in body)", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("QueryItems() expected errors for store failure, got none")
	}

	foundRetrieveError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "failed to retrieve items") {
			foundRetrieveError = true
			break
		}
	}
	if !foundRetrieveError {
		t.Errorf("QueryItems() expected error message containing 'failed to retrieve items', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_QueryItem_Exists(t *testing.T) {
	// Arrange
	ms := newMockStore()
	now := time.Now().UTC()
	ms.items["123"] = model.Item{ID: "123", Name: "Test Item", Description: "A test item", Price: 9.99, CreatedAt: now, UpdatedAt: now}
	router := setupGraphQLRouter(ms)

	query := `{ item(id: "123") { id name description price createdAt updatedAt } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("QueryItem() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		Item struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
			CreatedAt   string  `json:"createdAt"`
			UpdatedAt   string  `json:"updatedAt"`
		} `json:"item"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if data.Item.ID != "123" {
		t.Errorf("QueryItem() ID = %s, want 123", data.Item.ID)
	}
	if data.Item.Name != "Test Item" {
		t.Errorf("QueryItem() Name = %s, want 'Test Item'", data.Item.Name)
	}
	if data.Item.Description != "A test item" {
		t.Errorf("QueryItem() Description = %s, want 'A test item'", data.Item.Description)
	}
	if data.Item.Price != 9.99 {
		t.Errorf("QueryItem() Price = %f, want 9.99", data.Item.Price)
	}
	if data.Item.CreatedAt == "" {
		t.Error("QueryItem() CreatedAt should not be empty")
	}
	if data.Item.UpdatedAt == "" {
		t.Error("QueryItem() UpdatedAt should not be empty")
	}
}

func TestGraphQLHandler_QueryItem_NotFound(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	query := `{ item(id: "nonexistent") { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Not found returns an error in GraphQL
	if len(resp.Errors) == 0 {
		t.Fatal("QueryItem() expected errors for not found item, got none")
	}

	foundNotFoundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "item not found") {
			foundNotFoundError = true
			break
		}
	}
	if !foundNotFoundError {
		t.Errorf("QueryItem() expected error message containing 'item not found', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_QueryItem_InvalidID(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.getErr = store.ErrInvalidID
	router := setupGraphQLRouter(ms)

	query := `{ item(id: "invalid-id") { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("QueryItem() expected errors for invalid ID, got none")
	}

	foundInvalidIDError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "invalid item ID") {
			foundInvalidIDError = true
			break
		}
	}
	if !foundInvalidIDError {
		t.Errorf("QueryItem() expected error message containing 'invalid item ID', got: %v", resp.Errors)
	}
}

// --- Mutation Tests ---

func TestGraphQLHandler_CreateItem_Valid(t *testing.T) {
	// Arrange
	ms := newMockStore()
	now := time.Now().UTC()
	ms.createItem = &model.Item{
		ID:          "new-id",
		Name:        "New Item",
		Description: "A new item",
		Price:       19.99,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	router := setupGraphQLRouter(ms)

	query := `mutation { createItem(input: {name: "New Item", description: "A new item", price: 19.99}) { id name description price createdAt updatedAt } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("CreateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("CreateItem() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		CreateItem struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
			CreatedAt   string  `json:"createdAt"`
			UpdatedAt   string  `json:"updatedAt"`
		} `json:"createItem"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if data.CreateItem.ID != "new-id" {
		t.Errorf("CreateItem() ID = %s, want 'new-id'", data.CreateItem.ID)
	}
	if data.CreateItem.Name != "New Item" {
		t.Errorf("CreateItem() Name = %s, want 'New Item'", data.CreateItem.Name)
	}
	if data.CreateItem.Description != "A new item" {
		t.Errorf("CreateItem() Description = %s, want 'A new item'", data.CreateItem.Description)
	}
	if data.CreateItem.Price != 19.99 {
		t.Errorf("CreateItem() Price = %f, want 19.99", data.CreateItem.Price)
	}
}

func TestGraphQLHandler_CreateItem_EmptyName(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	query := `mutation { createItem(input: {name: "", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("CreateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("CreateItem() expected validation error for empty name, got none")
	}

	foundValidationError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "validation error") || strings.Contains(e.Message, "name cannot be empty") {
			foundValidationError = true
			break
		}
	}
	if !foundValidationError {
		t.Errorf("CreateItem() expected validation error message, got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_CreateItem_NegativePrice(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	query := `mutation { createItem(input: {name: "Test", price: -10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("CreateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("CreateItem() expected validation error for negative price, got none")
	}

	foundValidationError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "validation error") || strings.Contains(e.Message, "price cannot be negative") {
			foundValidationError = true
			break
		}
	}
	if !foundValidationError {
		t.Errorf("CreateItem() expected validation error message, got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_CreateItem_StoreError(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.createErr = errors.New("database write failed")
	router := setupGraphQLRouter(ms)

	query := `mutation { createItem(input: {name: "Test", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("CreateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("CreateItem() expected errors for store failure, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "internal server error") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("CreateItem() expected error message containing 'internal server error', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_UpdateItem_Valid(t *testing.T) {
	// Arrange
	ms := newMockStore()
	now := time.Now().UTC()
	ms.items["123"] = model.Item{ID: "123", Name: "Original", Price: 10.0, CreatedAt: now, UpdatedAt: now}
	ms.updateItem = &model.Item{
		ID:          "123",
		Name:        "Updated Item",
		Description: "Updated desc",
		Price:       29.99,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	router := setupGraphQLRouter(ms)

	query := `mutation { updateItem(id: "123", input: {name: "Updated Item", description: "Updated desc", price: 29.99}) { id name description price } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("UpdateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("UpdateItem() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		UpdateItem struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
		} `json:"updateItem"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if data.UpdateItem.ID != "123" {
		t.Errorf("UpdateItem() ID = %s, want '123'", data.UpdateItem.ID)
	}
	if data.UpdateItem.Name != "Updated Item" {
		t.Errorf("UpdateItem() Name = %s, want 'Updated Item'", data.UpdateItem.Name)
	}
	if data.UpdateItem.Description != "Updated desc" {
		t.Errorf("UpdateItem() Description = %s, want 'Updated desc'", data.UpdateItem.Description)
	}
	if data.UpdateItem.Price != 29.99 {
		t.Errorf("UpdateItem() Price = %f, want 29.99", data.UpdateItem.Price)
	}
}

func TestGraphQLHandler_UpdateItem_NotFound(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.updateErr = store.ErrNotFound
	router := setupGraphQLRouter(ms)

	query := `mutation { updateItem(id: "nonexistent", input: {name: "Test", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("UpdateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("UpdateItem() expected errors for not found, got none")
	}

	foundNotFoundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "item not found") {
			foundNotFoundError = true
			break
		}
	}
	if !foundNotFoundError {
		t.Errorf("UpdateItem() expected error message containing 'item not found', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_UpdateItem_EmptyName(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.items["123"] = model.Item{ID: "123", Name: "Original", Price: 10.0}
	router := setupGraphQLRouter(ms)

	query := `mutation { updateItem(id: "123", input: {name: "", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("UpdateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("UpdateItem() expected validation error for empty name, got none")
	}

	foundValidationError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "validation error") || strings.Contains(e.Message, "name cannot be empty") {
			foundValidationError = true
			break
		}
	}
	if !foundValidationError {
		t.Errorf("UpdateItem() expected validation error message, got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_UpdateItem_NegativePrice(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.items["123"] = model.Item{ID: "123", Name: "Original", Price: 10.0}
	router := setupGraphQLRouter(ms)

	query := `mutation { updateItem(id: "123", input: {name: "Test", price: -5.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("UpdateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("UpdateItem() expected validation error for negative price, got none")
	}

	foundValidationError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "validation error") || strings.Contains(e.Message, "price cannot be negative") {
			foundValidationError = true
			break
		}
	}
	if !foundValidationError {
		t.Errorf("UpdateItem() expected validation error message, got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_DeleteItem_Exists(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.items["123"] = model.Item{ID: "123", Name: "Test", Price: 10.0}
	router := setupGraphQLRouter(ms)

	query := `mutation { deleteItem(id: "123") }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("DeleteItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("DeleteItem() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		DeleteItem bool `json:"deleteItem"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if !data.DeleteItem {
		t.Error("DeleteItem() returned false, want true")
	}
}

func TestGraphQLHandler_DeleteItem_NotFound(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	query := `mutation { deleteItem(id: "nonexistent") }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("DeleteItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("DeleteItem() expected errors for not found, got none")
	}

	foundNotFoundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "item not found") {
			foundNotFoundError = true
			break
		}
	}
	if !foundNotFoundError {
		t.Errorf("DeleteItem() expected error message containing 'item not found', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_DeleteItem_StoreError(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.deleteErr = errors.New("database delete failed")
	router := setupGraphQLRouter(ms)

	query := `mutation { deleteItem(id: "123") }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("DeleteItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("DeleteItem() expected errors for store failure, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "internal server error") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("DeleteItem() expected error message containing 'internal server error', got: %v", resp.Errors)
	}
}

// --- Integration-style Tests ---

func TestGraphQLHandler_GraphiQLPlayground(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	// The graphql-go/handler library serves GraphiQL when Accept header indicates HTML
	req.Header.Set("Accept", "text/html")
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("GraphiQL() status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	// GraphiQL playground should return HTML content
	if !strings.Contains(body, "html") && !strings.Contains(body, "graphiql") && !strings.Contains(body, "GraphiQL") {
		t.Errorf("GraphiQL() response should contain HTML with GraphiQL, got body prefix=%q", body[:min(200, len(body))])
	}
}

func TestGraphQLHandler_InvalidJSON(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("this is not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Fatalf("InvalidJSON() status = %d, want 200 or 400", rr.Code)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		// If we can't decode as JSON, the handler returned an error page
		// which is acceptable behavior for invalid JSON
		return
	}

	// If we got a JSON response, it should have errors or null data
	if len(resp.Errors) == 0 && resp.Data == nil {
		t.Error("InvalidJSON() expected errors or null data")
	}
}

func TestGraphQLHandler_InvalidQuery(t *testing.T) {
	// Arrange
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	query := `{ nonExistentField { id } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("InvalidQuery() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("InvalidQuery() expected errors for invalid query, got none")
	}
}

// --- Additional Edge Case Tests for Coverage ---

func TestGraphQLHandler_CreateItem_AlreadyExists(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.createErr = store.ErrAlreadyExists
	router := setupGraphQLRouter(ms)

	query := `mutation { createItem(input: {name: "Test", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("CreateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("CreateItem() expected errors for already exists, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "item already exists") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("CreateItem() expected error message containing 'item already exists', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_UpdateItem_InvalidID(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.updateErr = store.ErrInvalidID
	router := setupGraphQLRouter(ms)

	query := `mutation { updateItem(id: "invalid", input: {name: "Test", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("UpdateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("UpdateItem() expected errors for invalid ID, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "invalid item ID") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("UpdateItem() expected error message containing 'invalid item ID', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_UpdateItem_StoreError(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.updateErr = errors.New("database update failed")
	router := setupGraphQLRouter(ms)

	query := `mutation { updateItem(id: "123", input: {name: "Test", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("UpdateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("UpdateItem() expected errors for store failure, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "internal server error") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("UpdateItem() expected error message containing 'internal server error', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_DeleteItem_InvalidID(t *testing.T) {
	// Arrange
	ms := newMockStore()
	ms.deleteErr = store.ErrInvalidID
	router := setupGraphQLRouter(ms)

	query := `mutation { deleteItem(id: "invalid") }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("DeleteItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("DeleteItem() expected errors for invalid ID, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "invalid item ID") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("DeleteItem() expected error message containing 'invalid item ID', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_QueryItems_SubsetFields(t *testing.T) {
	// Arrange - test querying only a subset of fields
	ms := newMockStore()
	now := time.Now().UTC()
	ms.items["1"] = model.Item{ID: "1", Name: "Item 1", Price: 10.0, CreatedAt: now, UpdatedAt: now}
	router := setupGraphQLRouter(ms)

	query := `{ items { id name price } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItems() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("QueryItems() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		Items []struct {
			ID    string  `json:"id"`
			Name  string  `json:"name"`
			Price float64 `json:"price"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if len(data.Items) != 1 {
		t.Fatalf("QueryItems() returned %d items, want 1", len(data.Items))
	}

	if data.Items[0].ID != "1" {
		t.Errorf("QueryItems() ID = %s, want '1'", data.Items[0].ID)
	}
	if data.Items[0].Name != "Item 1" {
		t.Errorf("QueryItems() Name = %s, want 'Item 1'", data.Items[0].Name)
	}
}

func TestGraphQLHandler_CreateItem_WithoutDescription(t *testing.T) {
	// Arrange - test creating item without optional description field
	ms := newMockStore()
	now := time.Now().UTC()
	ms.createItem = &model.Item{
		ID:        "new-id",
		Name:      "No Desc Item",
		Price:     5.0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	router := setupGraphQLRouter(ms)

	query := `mutation { createItem(input: {name: "No Desc Item", price: 5.0}) { id name description price } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("CreateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("CreateItem() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		CreateItem struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description *string `json:"description"`
			Price       float64 `json:"price"`
		} `json:"createItem"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if data.CreateItem.ID != "new-id" {
		t.Errorf("CreateItem() ID = %s, want 'new-id'", data.CreateItem.ID)
	}
	if data.CreateItem.Name != "No Desc Item" {
		t.Errorf("CreateItem() Name = %s, want 'No Desc Item'", data.CreateItem.Name)
	}
}

func TestGraphQLHandler_QueryItem_StoreError(t *testing.T) {
	// Arrange - test generic store error on item query
	ms := newMockStore()
	ms.getErr = errors.New("database read failed")
	router := setupGraphQLRouter(ms)

	query := `{ item(id: "123") { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("QueryItem() expected errors for store failure, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "internal server error") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("QueryItem() expected error message containing 'internal server error', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_CreateItem_InvalidIDError(t *testing.T) {
	// Arrange - test ErrInvalidID on create
	ms := newMockStore()
	ms.createErr = store.ErrInvalidID
	router := setupGraphQLRouter(ms)

	query := `mutation { createItem(input: {name: "Test", price: 10.0}) { id name } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("CreateItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("CreateItem() expected errors for invalid ID, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "invalid item ID") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("CreateItem() expected error message containing 'invalid item ID', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_DeleteItem_AlreadyExists(t *testing.T) {
	// Arrange - test ErrAlreadyExists on delete (unlikely but covers mapStoreError branch)
	ms := newMockStore()
	ms.deleteErr = store.ErrAlreadyExists
	router := setupGraphQLRouter(ms)

	query := `mutation { deleteItem(id: "123") }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("DeleteItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) == 0 {
		t.Fatal("DeleteItem() expected errors for already exists, got none")
	}

	foundError := false
	for _, e := range resp.Errors {
		if strings.Contains(e.Message, "item already exists") {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("DeleteItem() expected error message containing 'item already exists', got: %v", resp.Errors)
	}
}

func TestGraphQLHandler_QueryItem_CreatedAtUpdatedAtFormat(t *testing.T) {
	// Arrange - verify the time format is RFC3339
	ms := newMockStore()
	fixedTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	ms.items["time-test"] = model.Item{
		ID:        "time-test",
		Name:      "Time Test",
		Price:     1.0,
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
	router := setupGraphQLRouter(ms)

	query := `{ item(id: "time-test") { id createdAt updatedAt } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("QueryItem() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("QueryItem() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		Item struct {
			ID        string `json:"id"`
			CreatedAt string `json:"createdAt"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"item"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	expectedTime := "2025-06-15T10:30:00Z"
	if data.Item.CreatedAt != expectedTime {
		t.Errorf("QueryItem() CreatedAt = %s, want %s", data.Item.CreatedAt, expectedTime)
	}
	if data.Item.UpdatedAt != expectedTime {
		t.Errorf("QueryItem() UpdatedAt = %s, want %s", data.Item.UpdatedAt, expectedTime)
	}
}

func TestGraphQLHandler_EmptyQuery(t *testing.T) {
	// Arrange - test with empty query string
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query": ""}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert - empty query should return a valid response (possibly with errors)
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Fatalf("EmptyQuery() status = %d, want 200 or 400", rr.Code)
	}
}

func TestGraphQLHandler_IntrospectionQuery(t *testing.T) {
	// Arrange - test GraphQL introspection
	ms := newMockStore()
	router := setupGraphQLRouter(ms)

	query := `{ __schema { queryType { name } mutationType { name } } }`
	req := graphqlRequest(query)
	rr := httptest.NewRecorder()

	// Act
	router.ServeHTTP(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Fatalf("Introspection() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp graphqlResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(resp.Errors) > 0 {
		t.Fatalf("Introspection() unexpected errors: %v", resp.Errors)
	}

	var data struct {
		Schema struct {
			QueryType struct {
				Name string `json:"name"`
			} `json:"queryType"`
			MutationType struct {
				Name string `json:"name"`
			} `json:"mutationType"`
		} `json:"__schema"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if data.Schema.QueryType.Name != "Query" {
		t.Errorf("Introspection() queryType name = %s, want 'Query'", data.Schema.QueryType.Name)
	}
	if data.Schema.MutationType.Name != "Mutation" {
		t.Errorf("Introspection() mutationType name = %s, want 'Mutation'", data.Schema.MutationType.Name)
	}
}
