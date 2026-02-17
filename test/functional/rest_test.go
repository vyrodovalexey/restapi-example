//go:build functional

package functional

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// TestFunctional_REST_001_ListItemsEmptyStore tests listing items when store is empty.
// FT-REST-001: List items - empty store (GET /api/v1/items -> 200, empty array)
func TestFunctional_REST_001_ListItemsEmptyStore(t *testing.T) {
	LogTestStart(t, "FT-REST-001", "List items - empty store")
	defer LogTestEnd(t, "FT-REST-001")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	resp, err := client.Get(ctx, "/api/v1/items", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, apiResp)

	items, err := ParseItems(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse items: %v", err)
	}

	if len(items) != 0 {
		t.Errorf("Expected empty array, got %d items", len(items))
	}
}

// TestFunctional_REST_002_CreateItemValid tests creating a valid item.
// FT-REST-002: Create item - valid (POST /api/v1/items -> 201, created item)
func TestFunctional_REST_002_CreateItemValid(t *testing.T) {
	LogTestStart(t, "FT-REST-002", "Create item - valid")
	defer LogTestEnd(t, "FT-REST-002")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange
	createReq := CreateItemRequest{
		Name:        "Test Item",
		Description: "A test item for functional testing",
		Price:       19.99,
	}

	// Act
	resp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusCreated)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, apiResp)

	item, err := ParseItem(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse item: %v", err)
	}

	if item.ID == "" {
		t.Error("Expected item to have an ID")
	}
	if item.Name != createReq.Name {
		t.Errorf("Expected name %q, got %q", createReq.Name, item.Name)
	}
	if item.Description != createReq.Description {
		t.Errorf("Expected description %q, got %q", createReq.Description, item.Description)
	}
	if item.Price != createReq.Price {
		t.Errorf("Expected price %f, got %f", createReq.Price, item.Price)
	}
	if item.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	if item.UpdatedAt.IsZero() {
		t.Error("Expected UpdatedAt to be set")
	}
}

// TestFunctional_REST_003_CreateItemMissingName tests creating an item with missing name.
// FT-REST-003: Create item - missing name (POST -> 400, validation error)
func TestFunctional_REST_003_CreateItemMissingName(t *testing.T) {
	LogTestStart(t, "FT-REST-003", "Create item - missing name")
	defer LogTestEnd(t, "FT-REST-003")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - item with empty name
	createReq := CreateItemRequest{
		Name:        "",
		Description: "A test item",
		Price:       19.99,
	}

	// Act
	resp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusBadRequest)

	errResp, err := ParseErrorResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if errResp.Message != "name cannot be empty" {
		t.Errorf("Expected error message 'name cannot be empty', got %q", errResp.Message)
	}
}

// TestFunctional_REST_004_CreateItemNegativePrice tests creating an item with negative price.
// FT-REST-004: Create item - negative price (POST -> 400, validation error)
func TestFunctional_REST_004_CreateItemNegativePrice(t *testing.T) {
	LogTestStart(t, "FT-REST-004", "Create item - negative price")
	defer LogTestEnd(t, "FT-REST-004")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - item with negative price
	createReq := CreateItemRequest{
		Name:  "Test Item",
		Price: -10.00,
	}

	// Act
	resp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusBadRequest)

	errResp, err := ParseErrorResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if errResp.Message != "price cannot be negative" {
		t.Errorf("Expected error message 'price cannot be negative', got %q", errResp.Message)
	}
}

// TestFunctional_REST_005_CreateItemInvalidJSON tests creating an item with invalid JSON.
// FT-REST-005: Create item - invalid JSON (POST -> 400, invalid request)
func TestFunctional_REST_005_CreateItemInvalidJSON(t *testing.T) {
	LogTestStart(t, "FT-REST-005", "Create item - invalid JSON")
	defer LogTestEnd(t, "FT-REST-005")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - invalid JSON
	invalidJSON := "this is not valid json"

	// Act
	resp, err := client.Post(ctx, "/api/v1/items", invalidJSON, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusBadRequest)

	errResp, err := ParseErrorResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if errResp.Message != "invalid request body" {
		t.Errorf("Expected error message 'invalid request body', got %q", errResp.Message)
	}
}

// TestFunctional_REST_006_GetItemExists tests getting an existing item.
// FT-REST-006: Get item - exists (GET /api/v1/items/{id} -> 200, item)
func TestFunctional_REST_006_GetItemExists(t *testing.T) {
	LogTestStart(t, "FT-REST-006", "Get item - exists")
	defer LogTestEnd(t, "FT-REST-006")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create an item first
	createReq := CreateItemRequest{
		Name:        "Test Item",
		Description: "A test item",
		Price:       19.99,
	}

	createResp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	apiResp, err := ParseAPIResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	createdItem, err := ParseItem(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	// Act - get the created item
	resp, err := client.Get(ctx, "/api/v1/items/"+createdItem.ID, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	getApiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, getApiResp)

	item, err := ParseItem(getApiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse item: %v", err)
	}

	if item.ID != createdItem.ID {
		t.Errorf("Expected ID %q, got %q", createdItem.ID, item.ID)
	}
	if item.Name != createReq.Name {
		t.Errorf("Expected name %q, got %q", createReq.Name, item.Name)
	}
}

// TestFunctional_REST_007_GetItemNotFound tests getting a non-existent item.
// FT-REST-007: Get item - not found (GET -> 404, not found error)
func TestFunctional_REST_007_GetItemNotFound(t *testing.T) {
	LogTestStart(t, "FT-REST-007", "Get item - not found")
	defer LogTestEnd(t, "FT-REST-007")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act - try to get a non-existent item
	resp, err := client.Get(ctx, "/api/v1/items/non-existent-id-12345", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusNotFound)

	errResp, err := ParseErrorResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if errResp.Message != "item not found" {
		t.Errorf("Expected error message 'item not found', got %q", errResp.Message)
	}
}

// TestFunctional_REST_008_UpdateItemExists tests updating an existing item.
// FT-REST-008: Update item - exists (PUT /api/v1/items/{id} -> 200, updated item)
func TestFunctional_REST_008_UpdateItemExists(t *testing.T) {
	LogTestStart(t, "FT-REST-008", "Update item - exists")
	defer LogTestEnd(t, "FT-REST-008")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create an item first
	createReq := CreateItemRequest{
		Name:        "Original Item",
		Description: "Original description",
		Price:       19.99,
	}

	createResp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	apiResp, err := ParseAPIResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	createdItem, err := ParseItem(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	// Act - update the item
	updateReq := UpdateItemRequest{
		Name:        "Updated Item",
		Description: "Updated description",
		Price:       29.99,
	}

	resp, err := client.Put(ctx, "/api/v1/items/"+createdItem.ID, updateReq, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	updateApiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, updateApiResp)

	updatedItem, err := ParseItem(updateApiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse updated item: %v", err)
	}

	if updatedItem.ID != createdItem.ID {
		t.Errorf("Expected ID %q, got %q", createdItem.ID, updatedItem.ID)
	}
	if updatedItem.Name != updateReq.Name {
		t.Errorf("Expected name %q, got %q", updateReq.Name, updatedItem.Name)
	}
	if updatedItem.Description != updateReq.Description {
		t.Errorf("Expected description %q, got %q", updateReq.Description, updatedItem.Description)
	}
	if updatedItem.Price != updateReq.Price {
		t.Errorf("Expected price %f, got %f", updateReq.Price, updatedItem.Price)
	}
	if updatedItem.UpdatedAt.Before(createdItem.CreatedAt) || updatedItem.UpdatedAt.Equal(createdItem.CreatedAt) {
		t.Error("Expected UpdatedAt to be after CreatedAt")
	}
}

// TestFunctional_REST_009_UpdateItemNotFound tests updating a non-existent item.
// FT-REST-009: Update item - not found (PUT -> 404, not found error)
func TestFunctional_REST_009_UpdateItemNotFound(t *testing.T) {
	LogTestStart(t, "FT-REST-009", "Update item - not found")
	defer LogTestEnd(t, "FT-REST-009")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange
	updateReq := UpdateItemRequest{
		Name:  "Updated Item",
		Price: 29.99,
	}

	// Act - try to update a non-existent item
	resp, err := client.Put(ctx, "/api/v1/items/non-existent-id-12345", updateReq, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusNotFound)

	errResp, err := ParseErrorResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if errResp.Message != "item not found" {
		t.Errorf("Expected error message 'item not found', got %q", errResp.Message)
	}
}

// TestFunctional_REST_010_ListItemsWithData tests listing items when store has data.
// FT-REST-010: List items - with data (GET -> 200, array with items)
func TestFunctional_REST_010_ListItemsWithData(t *testing.T) {
	LogTestStart(t, "FT-REST-010", "List items - with data")
	defer LogTestEnd(t, "FT-REST-010")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create multiple items
	itemsToCreate := []CreateItemRequest{
		{Name: "Item 1", Description: "First item", Price: 10.00},
		{Name: "Item 2", Description: "Second item", Price: 20.00},
		{Name: "Item 3", Description: "Third item", Price: 30.00},
	}

	for _, item := range itemsToCreate {
		_, err := client.Post(ctx, "/api/v1/items", item, nil)
		if err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
	}

	// Act - list all items
	resp, err := client.Get(ctx, "/api/v1/items", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, apiResp)

	items, err := ParseItems(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse items: %v", err)
	}

	if len(items) != len(itemsToCreate) {
		t.Errorf("Expected %d items, got %d", len(itemsToCreate), len(items))
	}
}

// TestFunctional_REST_011_DeleteItemExists tests deleting an existing item.
// FT-REST-011: Delete item - exists (DELETE /api/v1/items/{id} -> 204)
func TestFunctional_REST_011_DeleteItemExists(t *testing.T) {
	LogTestStart(t, "FT-REST-011", "Delete item - exists")
	defer LogTestEnd(t, "FT-REST-011")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create an item first
	createReq := CreateItemRequest{
		Name:  "Item to Delete",
		Price: 19.99,
	}

	createResp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	apiResp, err := ParseAPIResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	createdItem, err := ParseItem(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	// Act - delete the item
	resp, err := client.Delete(ctx, "/api/v1/items/"+createdItem.ID, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusNoContent)

	// Verify item is deleted
	getResp, err := client.Get(ctx, "/api/v1/items/"+createdItem.ID, nil)
	if err != nil {
		t.Fatalf("Failed to verify deletion: %v", err)
	}

	AssertStatusCode(t, getResp, http.StatusNotFound)
}

// TestFunctional_REST_012_DeleteItemNotFound tests deleting a non-existent item.
// FT-REST-012: Delete item - not found (DELETE -> 404, not found error)
func TestFunctional_REST_012_DeleteItemNotFound(t *testing.T) {
	LogTestStart(t, "FT-REST-012", "Delete item - not found")
	defer LogTestEnd(t, "FT-REST-012")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act - try to delete a non-existent item
	resp, err := client.Delete(ctx, "/api/v1/items/non-existent-id-12345", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusNotFound)

	errResp, err := ParseErrorResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if errResp.Message != "item not found" {
		t.Errorf("Expected error message 'item not found', got %q", errResp.Message)
	}
}

// TestFunctional_REST_013_HealthCheck tests the health check endpoint.
// FT-REST-013: Health check (GET /health -> 200, healthy)
func TestFunctional_REST_013_HealthCheck(t *testing.T) {
	LogTestStart(t, "FT-REST-013", "Health check")
	defer LogTestEnd(t, "FT-REST-013")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	resp, err := client.Get(ctx, "/health", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, apiResp)

	health, err := ParseHealthResponse(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse health response: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got %q", health.Status)
	}

	if health.Version == "" {
		t.Error("Expected version to be set")
	}
}

// TestFunctional_REST_014_ReadinessCheck tests the readiness check endpoint.
// FT-REST-014: Readiness check (GET /ready -> 200, ready)
func TestFunctional_REST_014_ReadinessCheck(t *testing.T) {
	LogTestStart(t, "FT-REST-014", "Readiness check")
	defer LogTestEnd(t, "FT-REST-014")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	resp, err := client.Get(ctx, "/ready", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, apiResp)

	ready, err := ParseReadyResponse(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse ready response: %v", err)
	}

	if ready.Status != "ready" {
		t.Errorf("Expected status 'ready', got %q", ready.Status)
	}
}

// TestFunctional_REST_015_CRUDWorkflow tests the complete CRUD lifecycle.
// FT-REST-015: CRUD workflow - complete lifecycle
func TestFunctional_REST_015_CRUDWorkflow(t *testing.T) {
	LogTestStart(t, "FT-REST-015", "CRUD workflow - complete lifecycle")
	defer LogTestEnd(t, "FT-REST-015")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout*2)
	defer cancel()

	// Step 1: Create
	t.Log("Step 1: Create item")
	createReq := CreateItemRequest{
		Name:        "CRUD Test Item",
		Description: "Item for CRUD workflow test",
		Price:       49.99,
	}

	createResp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	AssertStatusCode(t, createResp, http.StatusCreated)

	createApiResp, err := ParseAPIResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}
	AssertSuccess(t, createApiResp)

	createdItem, err := ParseItem(createApiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	itemID := createdItem.ID
	t.Logf("Created item with ID: %s", itemID)

	// Step 2: Read
	t.Log("Step 2: Read item")
	readResp, err := client.Get(ctx, "/api/v1/items/"+itemID, nil)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	AssertStatusCode(t, readResp, http.StatusOK)

	readApiResp, err := ParseAPIResponse(readResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse read response: %v", err)
	}
	AssertSuccess(t, readApiResp)

	readItem, err := ParseItem(readApiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse read item: %v", err)
	}

	if readItem.Name != createReq.Name {
		t.Errorf("Read item name mismatch: expected %q, got %q", createReq.Name, readItem.Name)
	}

	// Step 3: Update
	t.Log("Step 3: Update item")
	updateReq := UpdateItemRequest{
		Name:        "Updated CRUD Test Item",
		Description: "Updated description",
		Price:       59.99,
	}

	updateResp, err := client.Put(ctx, "/api/v1/items/"+itemID, updateReq, nil)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	AssertStatusCode(t, updateResp, http.StatusOK)

	updateApiResp, err := ParseAPIResponse(updateResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse update response: %v", err)
	}
	AssertSuccess(t, updateApiResp)

	// Step 4: Verify Update
	t.Log("Step 4: Verify update")
	verifyResp, err := client.Get(ctx, "/api/v1/items/"+itemID, nil)
	if err != nil {
		t.Fatalf("Verify update failed: %v", err)
	}
	AssertStatusCode(t, verifyResp, http.StatusOK)

	verifyApiResp, err := ParseAPIResponse(verifyResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse verify response: %v", err)
	}

	verifyItem, err := ParseItem(verifyApiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse verify item: %v", err)
	}

	if verifyItem.Name != updateReq.Name {
		t.Errorf("Updated item name mismatch: expected %q, got %q", updateReq.Name, verifyItem.Name)
	}
	if verifyItem.Price != updateReq.Price {
		t.Errorf("Updated item price mismatch: expected %f, got %f", updateReq.Price, verifyItem.Price)
	}

	// Step 5: Delete
	t.Log("Step 5: Delete item")
	deleteResp, err := client.Delete(ctx, "/api/v1/items/"+itemID, nil)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	AssertStatusCode(t, deleteResp, http.StatusNoContent)

	// Step 6: Verify Delete
	t.Log("Step 6: Verify delete")
	verifyDeleteResp, err := client.Get(ctx, "/api/v1/items/"+itemID, nil)
	if err != nil {
		t.Fatalf("Verify delete failed: %v", err)
	}
	AssertStatusCode(t, verifyDeleteResp, http.StatusNotFound)

	t.Log("CRUD workflow completed successfully")
}

// TestFunctional_REST_016_ConcurrentCreates tests concurrent item creation.
// FT-REST-016: Concurrent creates (10 concurrent requests)
func TestFunctional_REST_016_ConcurrentCreates(t *testing.T) {
	LogTestStart(t, "FT-REST-016", "Concurrent creates")
	defer LogTestEnd(t, "FT-REST-016")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout*2)
	defer cancel()

	const numConcurrent = 10
	var wg sync.WaitGroup
	results := make(chan *Response, numConcurrent)
	errors := make(chan error, numConcurrent)

	// Launch concurrent requests
	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			createReq := CreateItemRequest{
				Name:        "Concurrent Item " + time.Now().Format(time.RFC3339Nano),
				Description: "Created concurrently",
				Price:       float64(index) * 10.0,
			}

			resp, err := client.Post(ctx, "/api/v1/items", createReq, nil)
			if err != nil {
				errors <- err
				return
			}
			results <- resp
		}(i)
	}

	// Wait for all requests to complete
	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent request failed: %v", err)
	}

	// Verify all requests succeeded
	successCount := 0
	for resp := range results {
		if resp.StatusCode == http.StatusCreated {
			successCount++
		} else {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}
	}

	if successCount != numConcurrent {
		t.Errorf("Expected %d successful creates, got %d", numConcurrent, successCount)
	}

	// Verify all items were created
	listResp, err := client.Get(ctx, "/api/v1/items", nil)
	if err != nil {
		t.Fatalf("Failed to list items: %v", err)
	}

	apiResp, err := ParseAPIResponse(listResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}

	items, err := ParseItems(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse items: %v", err)
	}

	if len(items) != numConcurrent {
		t.Errorf("Expected %d items in store, got %d", numConcurrent, len(items))
	}
}

// TestFunctional_REST_017_RequestWithXRequestID tests X-Request-ID header handling.
// FT-REST-017: Request with X-Request-ID header
func TestFunctional_REST_017_RequestWithXRequestID(t *testing.T) {
	LogTestStart(t, "FT-REST-017", "Request with X-Request-ID header")
	defer LogTestEnd(t, "FT-REST-017")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange
	requestID := "test-request-id-12345"
	headers := map[string]string{
		"X-Request-ID": requestID,
	}

	// Act
	resp, err := client.Get(ctx, "/health", headers)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)
	AssertHeader(t, resp, "X-Request-ID", requestID)
}

// TestFunctional_REST_RequestIDGenerated tests that X-Request-ID is generated when not provided.
func TestFunctional_REST_RequestIDGenerated(t *testing.T) {
	LogTestStart(t, "FT-REST-EXTRA", "Request ID generated when not provided")
	defer LogTestEnd(t, "FT-REST-EXTRA")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act - request without X-Request-ID header
	resp, err := client.Get(ctx, "/health", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	generatedID := resp.Headers.Get("X-Request-ID")
	if generatedID == "" {
		t.Error("Expected X-Request-ID to be generated")
	}

	t.Logf("Generated X-Request-ID: %s", generatedID)
}

// TestFunctional_REST_ContentTypeJSON tests that responses have correct Content-Type.
func TestFunctional_REST_ContentTypeJSON(t *testing.T) {
	LogTestStart(t, "FT-REST-EXTRA", "Content-Type is application/json")
	defer LogTestEnd(t, "FT-REST-EXTRA")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Test various endpoints
	endpoints := []string{
		"/health",
		"/api/v1/items",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			resp, err := client.Get(ctx, endpoint, nil)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			contentType := resp.Headers.Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type 'application/json', got %q", contentType)
			}
		})
	}
}

// testAPIKey is the API key used in auth functional tests.
const testAPIKey = "test-api-key-12345"

// testAPIKeyConfig is the API key config string for test servers.
const testAPIKeyConfig = testAPIKey + ":test-service"

// testBasicUser is the username for basic auth tests.
const testBasicUser = "testuser"

// testBasicPassword is the password for basic auth tests.
const testBasicPassword = "testpass123"

// generateTestBasicAuthConfig creates a basic auth config string with
// a bcrypt-hashed password for the test user.
func generateTestBasicAuthConfig(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(testBasicPassword), bcrypt.MinCost,
	)
	if err != nil {
		t.Fatalf("Failed to generate bcrypt hash: %v", err)
	}
	return testBasicUser + ":" + string(hash)
}

// TestReadyEndpoint tests the GET /ready endpoint.
func TestReadyEndpoint(t *testing.T) {
	LogTestStart(t, "FT-AUTH-READY", "Ready endpoint")
	defer LogTestEnd(t, "FT-AUTH-READY")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	// Act
	resp, err := client.Get(ctx, "/ready", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	AssertSuccess(t, apiResp)

	ready, err := ParseReadyResponse(apiResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse ready response: %v", err)
	}

	if ready.Status != "ready" {
		t.Errorf("Expected status 'ready', got %q", ready.Status)
	}
}

// TestAuthPublicEndpoints tests that public endpoints are accessible
// without authentication even when auth is enabled.
func TestAuthPublicEndpoints(t *testing.T) {
	LogTestStart(t, "FT-AUTH-001", "Public endpoints accessible without auth")
	defer LogTestEnd(t, "FT-AUTH-001")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	tests := []struct {
		name string
		path string
	}{
		{name: "health endpoint", path: "/health"},
		{name: "ready endpoint", path: "/ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act - no auth headers
			resp, err := client.Get(ctx, tt.path, nil)
			if err != nil {
				t.Fatalf("Request to %s failed: %v", tt.path, err)
			}

			// Assert - should be accessible without auth
			AssertStatusCode(t, resp, http.StatusOK)

			apiResp, err := ParseAPIResponse(resp.Body)
			if err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}
			AssertSuccess(t, apiResp)
		})
	}
}

// TestAuthProtectedEndpointsRequireAuth tests that protected endpoints
// return 401 without authentication.
func TestAuthProtectedEndpointsRequireAuth(t *testing.T) {
	LogTestStart(t, "FT-AUTH-002", "Protected endpoints require auth")
	defer LogTestEnd(t, "FT-AUTH-002")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	// Act - request without auth
	resp, err := client.Get(ctx, "/api/v1/items", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert - should return 401
	AssertStatusCode(t, resp, http.StatusUnauthorized)
}

// TestAuthAPIKeyAccess tests that protected endpoints are accessible
// with a valid API key.
func TestAuthAPIKeyAccess(t *testing.T) {
	LogTestStart(t, "FT-AUTH-003", "API key access")
	defer LogTestEnd(t, "FT-AUTH-003")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	// Act - request with valid API key
	headers := APIKeyHeaders(testAPIKey)
	resp, err := client.Get(ctx, "/api/v1/items", headers)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert - should be accessible
	AssertStatusCode(t, resp, http.StatusOK)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	AssertSuccess(t, apiResp)
}

// TestAuthBasicAccess tests that protected endpoints are accessible
// with valid basic auth credentials.
func TestAuthBasicAccess(t *testing.T) {
	LogTestStart(t, "FT-AUTH-004", "Basic auth access")
	defer LogTestEnd(t, "FT-AUTH-004")

	usersConfig := generateTestBasicAuthConfig(t)
	ts := NewTestServerWithBasicAuth(t, usersConfig)
	ts.Start()
	defer ts.Stop()

	basicClient := NewBasicAuthClient(
		t, ts.BaseURL, testBasicUser, testBasicPassword,
	)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	// Act - request with valid basic auth
	resp, err := basicClient.DoWithBasicAuth(ctx, Request{
		Method: http.MethodGet,
		Path:   "/api/v1/items",
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert - should be accessible
	AssertStatusCode(t, resp, http.StatusOK)

	apiResp, err := ParseAPIResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	AssertSuccess(t, apiResp)
}

// TestAuthInvalidAPIKey tests that an invalid API key returns 401.
func TestAuthInvalidAPIKey(t *testing.T) {
	LogTestStart(t, "FT-AUTH-005", "Invalid API key")
	defer LogTestEnd(t, "FT-AUTH-005")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	// Act - request with invalid API key
	headers := APIKeyHeaders("invalid-api-key-99999")
	resp, err := client.Get(ctx, "/api/v1/items", headers)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert - should return 401
	AssertStatusCode(t, resp, http.StatusUnauthorized)
}

// TestAuthInvalidBasicAuth tests that wrong basic auth credentials
// return 401.
func TestAuthInvalidBasicAuth(t *testing.T) {
	LogTestStart(t, "FT-AUTH-006", "Invalid basic auth")
	defer LogTestEnd(t, "FT-AUTH-006")

	usersConfig := generateTestBasicAuthConfig(t)
	ts := NewTestServerWithBasicAuth(t, usersConfig)
	ts.Start()
	defer ts.Stop()

	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	tests := []struct {
		name     string
		username string
		password string
	}{
		{
			name:     "wrong password",
			username: testBasicUser,
			password: "wrongpassword",
		},
		{
			name:     "unknown user",
			username: "unknownuser",
			password: testBasicPassword,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basicClient := NewBasicAuthClient(
				t, ts.BaseURL, tt.username, tt.password,
			)

			// Act
			resp, err := basicClient.DoWithBasicAuth(ctx, Request{
				Method: http.MethodGet,
				Path:   "/api/v1/items",
			})
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			// Assert
			AssertStatusCode(t, resp, http.StatusUnauthorized)
		})
	}
}

// TestAuthCRUDWithAPIKey tests the full CRUD workflow with API key
// authentication.
func TestAuthCRUDWithAPIKey(t *testing.T) {
	LogTestStart(t, "FT-AUTH-007", "CRUD with API key auth")
	defer LogTestEnd(t, "FT-AUTH-007")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout*2,
	)
	defer cancel()

	headers := APIKeyHeaders(testAPIKey)

	// Step 1: Create
	t.Log("Step 1: Create item with API key")
	createReq := CreateItemRequest{
		Name:        "Auth CRUD Test Item",
		Description: "Item for auth CRUD workflow test",
		Price:       49.99,
	}

	createResp, err := client.Post(
		ctx, "/api/v1/items", createReq, headers,
	)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	AssertStatusCode(t, createResp, http.StatusCreated)

	createAPIResp, err := ParseAPIResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}
	AssertSuccess(t, createAPIResp)

	createdItem, err := ParseItem(createAPIResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	itemID := createdItem.ID
	t.Logf("Created item with ID: %s", itemID)

	// Step 2: Read
	t.Log("Step 2: Read item with API key")
	readResp, err := client.Get(
		ctx, "/api/v1/items/"+itemID, headers,
	)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	AssertStatusCode(t, readResp, http.StatusOK)

	readAPIResp, err := ParseAPIResponse(readResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse read response: %v", err)
	}
	AssertSuccess(t, readAPIResp)

	readItem, err := ParseItem(readAPIResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse read item: %v", err)
	}
	if readItem.Name != createReq.Name {
		t.Errorf(
			"Read item name mismatch: expected %q, got %q",
			createReq.Name, readItem.Name,
		)
	}

	// Step 3: Update
	t.Log("Step 3: Update item with API key")
	updateReq := UpdateItemRequest{
		Name:        "Updated Auth CRUD Item",
		Description: "Updated description",
		Price:       59.99,
	}

	updateResp, err := client.Put(
		ctx, "/api/v1/items/"+itemID, updateReq, headers,
	)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	AssertStatusCode(t, updateResp, http.StatusOK)

	// Step 4: List
	t.Log("Step 4: List items with API key")
	listResp, err := client.Get(ctx, "/api/v1/items", headers)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	AssertStatusCode(t, listResp, http.StatusOK)

	listAPIResp, err := ParseAPIResponse(listResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}
	AssertSuccess(t, listAPIResp)

	items, err := ParseItems(listAPIResp.Data)
	if err != nil {
		t.Fatalf("Failed to parse items: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}

	// Step 5: Delete
	t.Log("Step 5: Delete item with API key")
	deleteResp, err := client.Delete(
		ctx, "/api/v1/items/"+itemID, headers,
	)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	AssertStatusCode(t, deleteResp, http.StatusNoContent)

	// Step 6: Verify deletion
	t.Log("Step 6: Verify deletion with API key")
	verifyResp, err := client.Get(
		ctx, "/api/v1/items/"+itemID, headers,
	)
	if err != nil {
		t.Fatalf("Verify delete failed: %v", err)
	}
	AssertStatusCode(t, verifyResp, http.StatusNotFound)

	t.Log("Auth CRUD workflow completed successfully")
}
