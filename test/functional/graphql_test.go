//go:build functional

package functional

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// GraphQLResponse represents a standard GraphQL JSON response.
type GraphQLResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// ParseGraphQLResponse parses a GraphQL response from bytes.
func ParseGraphQLResponse(body []byte) (*GraphQLResponse, error) {
	var resp GraphQLResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}
	return &resp, nil
}

// graphqlQuery sends a GraphQL query via POST to /graphql.
func graphqlQuery(ctx context.Context, client *HTTPClient, query string) (*Response, error) {
	body := map[string]string{"query": query}
	return client.Post(ctx, "/graphql", body, nil)
}

// graphqlQueryWithHeaders sends a GraphQL query via POST to /graphql with custom headers.
func graphqlQueryWithHeaders(ctx context.Context, client *HTTPClient, query string, headers map[string]string) (*Response, error) {
	body := map[string]string{"query": query}
	return client.Post(ctx, "/graphql", body, headers)
}

// --- Query Tests ---

// TestFunctional_GQL_001_QueryItemsEmpty tests querying items from an empty store.
// FT-GQL-001: Query items - empty store returns empty array
func TestFunctional_GQL_001_QueryItemsEmpty(t *testing.T) {
	LogTestStart(t, "FT-GQL-001", "Query items - empty store")
	defer LogTestEnd(t, "FT-GQL-001")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	query := `{ items { id name description price createdAt updatedAt } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) > 0 {
		t.Fatalf("Unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	var data struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(gqlResp.Data["items"], &data.Items); err != nil {
		// Try parsing the full data object
		rawData, _ := json.Marshal(gqlResp.Data)
		if err2 := json.Unmarshal(rawData, &data); err2 != nil {
			t.Fatalf("Failed to parse items data: %v (raw: %s)", err2, string(resp.Body))
		}
	}

	if len(data.Items) != 0 {
		t.Errorf("Expected empty items array, got %d items", len(data.Items))
	}
}

// TestFunctional_GQL_002_QueryItemsWithData tests querying items after creating some.
// FT-GQL-002: Query items - with data returns all items
func TestFunctional_GQL_002_QueryItemsWithData(t *testing.T) {
	LogTestStart(t, "FT-GQL-002", "Query items - with data")
	defer LogTestEnd(t, "FT-GQL-002")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create items via GraphQL mutations
	items := []struct {
		name  string
		price float64
	}{
		{"GQL Item 1", 10.50},
		{"GQL Item 2", 20.00},
		{"GQL Item 3", 30.75},
	}

	for _, item := range items {
		createQuery := fmt.Sprintf(
			`mutation { createItem(input: {name: "%s", price: %.2f}) { id } }`,
			item.name, item.price,
		)
		createResp, err := graphqlQuery(ctx, client, createQuery)
		if err != nil {
			t.Fatalf("Failed to create item: %v", err)
		}
		AssertStatusCode(t, createResp, http.StatusOK)
	}

	// Act - query all items
	query := `{ items { id name price } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) > 0 {
		t.Fatalf("Unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	// Parse items from the data
	rawData, _ := json.Marshal(gqlResp.Data)
	var data struct {
		Items []struct {
			ID    string  `json:"id"`
			Name  string  `json:"name"`
			Price float64 `json:"price"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rawData, &data); err != nil {
		t.Fatalf("Failed to parse items: %v", err)
	}

	if len(data.Items) != len(items) {
		t.Errorf("Expected %d items, got %d", len(items), len(data.Items))
	}
}

// TestFunctional_GQL_003_QueryItemByID tests querying a single item by ID.
// FT-GQL-003: Query item by ID returns correct item
func TestFunctional_GQL_003_QueryItemByID(t *testing.T) {
	LogTestStart(t, "FT-GQL-003", "Query item by ID")
	defer LogTestEnd(t, "FT-GQL-003")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create an item
	createQuery := `mutation { createItem(input: {name: "Query Test Item", description: "For query test", price: 25.50}) { id name description price } }`
	createResp, err := graphqlQuery(ctx, client, createQuery)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	createGQL, err := ParseGraphQLResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	rawCreateData, _ := json.Marshal(createGQL.Data)
	var createData struct {
		CreateItem struct {
			ID string `json:"id"`
		} `json:"createItem"`
	}
	if err := json.Unmarshal(rawCreateData, &createData); err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	itemID := createData.CreateItem.ID
	if itemID == "" {
		t.Fatal("Created item has empty ID")
	}
	t.Logf("Created item with ID: %s", itemID)

	// Act - query item by ID
	getQuery := fmt.Sprintf(`{ item(id: "%s") { id name description price createdAt updatedAt } }`, itemID)
	resp, err := graphqlQuery(ctx, client, getQuery)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) > 0 {
		t.Fatalf("Unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	rawData, _ := json.Marshal(gqlResp.Data)
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
	if err := json.Unmarshal(rawData, &data); err != nil {
		t.Fatalf("Failed to parse item data: %v", err)
	}

	if data.Item.ID != itemID {
		t.Errorf("Expected ID %q, got %q", itemID, data.Item.ID)
	}
	if data.Item.Name != "Query Test Item" {
		t.Errorf("Expected name %q, got %q", "Query Test Item", data.Item.Name)
	}
	if data.Item.Description != "For query test" {
		t.Errorf("Expected description %q, got %q", "For query test", data.Item.Description)
	}
	if data.Item.Price != 25.50 {
		t.Errorf("Expected price %f, got %f", 25.50, data.Item.Price)
	}
	if data.Item.CreatedAt == "" {
		t.Error("Expected createdAt to be set")
	}
	if data.Item.UpdatedAt == "" {
		t.Error("Expected updatedAt to be set")
	}
}

// TestFunctional_GQL_004_QueryItemNotFound tests querying a non-existent item.
// FT-GQL-004: Query non-existent item returns error
func TestFunctional_GQL_004_QueryItemNotFound(t *testing.T) {
	LogTestStart(t, "FT-GQL-004", "Query item - not found")
	defer LogTestEnd(t, "FT-GQL-004")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act - query non-existent item
	query := `{ item(id: "non-existent-id-12345") { id name } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert - GraphQL always returns 200, errors are in the body
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	// Not found should return an error in the errors array
	if len(gqlResp.Errors) == 0 {
		t.Fatal("Expected GraphQL errors for non-existent item, got none")
	}

	foundNotFound := false
	for _, e := range gqlResp.Errors {
		if strings.Contains(e.Message, "item not found") {
			foundNotFound = true
			break
		}
	}
	if !foundNotFound {
		t.Errorf("Expected error containing 'item not found', got: %v", gqlResp.Errors)
	}
}

// --- Mutation Tests ---

// TestFunctional_GQL_005_CreateItemValid tests creating an item with valid input.
// FT-GQL-005: Create item - valid input returns created item
func TestFunctional_GQL_005_CreateItemValid(t *testing.T) {
	LogTestStart(t, "FT-GQL-005", "Create item - valid")
	defer LogTestEnd(t, "FT-GQL-005")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	query := `mutation { createItem(input: {name: "GraphQL Created Item", description: "Created via GraphQL", price: 42.99}) { id name description price createdAt updatedAt } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) > 0 {
		t.Fatalf("Unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	rawData, _ := json.Marshal(gqlResp.Data)
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
	if err := json.Unmarshal(rawData, &data); err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	if data.CreateItem.ID == "" {
		t.Error("Expected created item to have an ID")
	}
	if data.CreateItem.Name != "GraphQL Created Item" {
		t.Errorf("Expected name %q, got %q", "GraphQL Created Item", data.CreateItem.Name)
	}
	if data.CreateItem.Description != "Created via GraphQL" {
		t.Errorf("Expected description %q, got %q", "Created via GraphQL", data.CreateItem.Description)
	}
	if data.CreateItem.Price != 42.99 {
		t.Errorf("Expected price %f, got %f", 42.99, data.CreateItem.Price)
	}
	if data.CreateItem.CreatedAt == "" {
		t.Error("Expected createdAt to be set")
	}
	if data.CreateItem.UpdatedAt == "" {
		t.Error("Expected updatedAt to be set")
	}
}

// TestFunctional_GQL_006_CreateItemEmptyName tests creating an item with empty name.
// FT-GQL-006: Create item - empty name returns validation error
func TestFunctional_GQL_006_CreateItemEmptyName(t *testing.T) {
	LogTestStart(t, "FT-GQL-006", "Create item - empty name")
	defer LogTestEnd(t, "FT-GQL-006")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	query := `mutation { createItem(input: {name: "", price: 10.00}) { id name } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert - GraphQL returns 200 even for errors
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) == 0 {
		t.Fatal("Expected validation error for empty name, got none")
	}

	foundValidationError := false
	for _, e := range gqlResp.Errors {
		if strings.Contains(e.Message, "validation error") || strings.Contains(e.Message, "name cannot be empty") {
			foundValidationError = true
			break
		}
	}
	if !foundValidationError {
		t.Errorf("Expected validation error message, got: %v", gqlResp.Errors)
	}
}

// TestFunctional_GQL_007_CreateItemNegativePrice tests creating an item with negative price.
// FT-GQL-007: Create item - negative price returns validation error
func TestFunctional_GQL_007_CreateItemNegativePrice(t *testing.T) {
	LogTestStart(t, "FT-GQL-007", "Create item - negative price")
	defer LogTestEnd(t, "FT-GQL-007")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	query := `mutation { createItem(input: {name: "Negative Price Item", price: -10.00}) { id name } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) == 0 {
		t.Fatal("Expected validation error for negative price, got none")
	}

	foundValidationError := false
	for _, e := range gqlResp.Errors {
		if strings.Contains(e.Message, "validation error") || strings.Contains(e.Message, "price cannot be negative") {
			foundValidationError = true
			break
		}
	}
	if !foundValidationError {
		t.Errorf("Expected validation error message, got: %v", gqlResp.Errors)
	}
}

// TestFunctional_GQL_008_UpdateItemValid tests updating an existing item.
// FT-GQL-008: Update item - valid returns updated item
func TestFunctional_GQL_008_UpdateItemValid(t *testing.T) {
	LogTestStart(t, "FT-GQL-008", "Update item - valid")
	defer LogTestEnd(t, "FT-GQL-008")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create an item first
	createQuery := `mutation { createItem(input: {name: "Original GQL Item", description: "Original", price: 15.00}) { id } }`
	createResp, err := graphqlQuery(ctx, client, createQuery)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	createGQL, err := ParseGraphQLResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	rawCreateData, _ := json.Marshal(createGQL.Data)
	var createData struct {
		CreateItem struct {
			ID string `json:"id"`
		} `json:"createItem"`
	}
	if err := json.Unmarshal(rawCreateData, &createData); err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	itemID := createData.CreateItem.ID
	t.Logf("Created item with ID: %s", itemID)

	// Act - update the item
	updateQuery := fmt.Sprintf(
		`mutation { updateItem(id: "%s", input: {name: "Updated GQL Item", description: "Updated via GraphQL", price: 35.00}) { id name description price } }`,
		itemID,
	)
	resp, err := graphqlQuery(ctx, client, updateQuery)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) > 0 {
		t.Fatalf("Unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	rawData, _ := json.Marshal(gqlResp.Data)
	var data struct {
		UpdateItem struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
		} `json:"updateItem"`
	}
	if err := json.Unmarshal(rawData, &data); err != nil {
		t.Fatalf("Failed to parse updated item: %v", err)
	}

	if data.UpdateItem.ID != itemID {
		t.Errorf("Expected ID %q, got %q", itemID, data.UpdateItem.ID)
	}
	if data.UpdateItem.Name != "Updated GQL Item" {
		t.Errorf("Expected name %q, got %q", "Updated GQL Item", data.UpdateItem.Name)
	}
	if data.UpdateItem.Description != "Updated via GraphQL" {
		t.Errorf("Expected description %q, got %q", "Updated via GraphQL", data.UpdateItem.Description)
	}
	if data.UpdateItem.Price != 35.00 {
		t.Errorf("Expected price %f, got %f", 35.00, data.UpdateItem.Price)
	}
}

// TestFunctional_GQL_009_UpdateItemNotFound tests updating a non-existent item.
// FT-GQL-009: Update non-existent item returns error
func TestFunctional_GQL_009_UpdateItemNotFound(t *testing.T) {
	LogTestStart(t, "FT-GQL-009", "Update item - not found")
	defer LogTestEnd(t, "FT-GQL-009")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	query := `mutation { updateItem(id: "non-existent-id-12345", input: {name: "Updated", price: 10.00}) { id name } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) == 0 {
		t.Fatal("Expected error for non-existent item update, got none")
	}

	foundNotFound := false
	for _, e := range gqlResp.Errors {
		if strings.Contains(e.Message, "item not found") {
			foundNotFound = true
			break
		}
	}
	if !foundNotFound {
		t.Errorf("Expected error containing 'item not found', got: %v", gqlResp.Errors)
	}
}

// TestFunctional_GQL_010_DeleteItemValid tests deleting an existing item.
// FT-GQL-010: Delete existing item returns true
func TestFunctional_GQL_010_DeleteItemValid(t *testing.T) {
	LogTestStart(t, "FT-GQL-010", "Delete item - valid")
	defer LogTestEnd(t, "FT-GQL-010")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Arrange - create an item first
	createQuery := `mutation { createItem(input: {name: "Item to Delete", price: 5.00}) { id } }`
	createResp, err := graphqlQuery(ctx, client, createQuery)
	if err != nil {
		t.Fatalf("Failed to create item: %v", err)
	}

	createGQL, err := ParseGraphQLResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	rawCreateData, _ := json.Marshal(createGQL.Data)
	var createData struct {
		CreateItem struct {
			ID string `json:"id"`
		} `json:"createItem"`
	}
	if err := json.Unmarshal(rawCreateData, &createData); err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	itemID := createData.CreateItem.ID
	t.Logf("Created item with ID: %s", itemID)

	// Act - delete the item
	deleteQuery := fmt.Sprintf(`mutation { deleteItem(id: "%s") }`, itemID)
	resp, err := graphqlQuery(ctx, client, deleteQuery)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) > 0 {
		t.Fatalf("Unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	rawData, _ := json.Marshal(gqlResp.Data)
	var data struct {
		DeleteItem bool `json:"deleteItem"`
	}
	if err := json.Unmarshal(rawData, &data); err != nil {
		t.Fatalf("Failed to parse delete response: %v", err)
	}

	if !data.DeleteItem {
		t.Error("Expected deleteItem to return true")
	}
}

// TestFunctional_GQL_011_DeleteItemNotFound tests deleting a non-existent item.
// FT-GQL-011: Delete non-existent item returns error
func TestFunctional_GQL_011_DeleteItemNotFound(t *testing.T) {
	LogTestStart(t, "FT-GQL-011", "Delete item - not found")
	defer LogTestEnd(t, "FT-GQL-011")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act
	query := `mutation { deleteItem(id: "non-existent-id-12345") }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) == 0 {
		t.Fatal("Expected error for non-existent item deletion, got none")
	}

	foundNotFound := false
	for _, e := range gqlResp.Errors {
		if strings.Contains(e.Message, "item not found") {
			foundNotFound = true
			break
		}
	}
	if !foundNotFound {
		t.Errorf("Expected error containing 'item not found', got: %v", gqlResp.Errors)
	}
}

// --- Workflow Tests ---

// TestFunctional_GQL_012_CRUDWorkflow tests the complete CRUD lifecycle via GraphQL.
// FT-GQL-012: CRUD workflow - create, query, update, verify, delete, verify
func TestFunctional_GQL_012_CRUDWorkflow(t *testing.T) {
	LogTestStart(t, "FT-GQL-012", "CRUD workflow via GraphQL")
	defer LogTestEnd(t, "FT-GQL-012")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout*2)
	defer cancel()

	// Step 1: Create item
	t.Log("Step 1: Create item via GraphQL")
	createQuery := `mutation { createItem(input: {name: "CRUD Workflow Item", description: "For CRUD test", price: 49.99}) { id name description price createdAt updatedAt } }`
	createResp, err := graphqlQuery(ctx, client, createQuery)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	AssertStatusCode(t, createResp, http.StatusOK)

	createGQL, err := ParseGraphQLResponse(createResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}
	if len(createGQL.Errors) > 0 {
		t.Fatalf("Create returned errors: %v", createGQL.Errors)
	}

	rawCreateData, _ := json.Marshal(createGQL.Data)
	var createData struct {
		CreateItem struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
		} `json:"createItem"`
	}
	if err := json.Unmarshal(rawCreateData, &createData); err != nil {
		t.Fatalf("Failed to parse created item: %v", err)
	}

	itemID := createData.CreateItem.ID
	if itemID == "" {
		t.Fatal("Created item has empty ID")
	}
	t.Logf("Created item with ID: %s", itemID)

	if createData.CreateItem.Name != "CRUD Workflow Item" {
		t.Errorf("Expected name %q, got %q", "CRUD Workflow Item", createData.CreateItem.Name)
	}

	// Step 2: Query item by ID
	t.Log("Step 2: Query item by ID")
	getQuery := fmt.Sprintf(`{ item(id: "%s") { id name description price } }`, itemID)
	getResp, err := graphqlQuery(ctx, client, getQuery)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	AssertStatusCode(t, getResp, http.StatusOK)

	getGQL, err := ParseGraphQLResponse(getResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse query response: %v", err)
	}
	if len(getGQL.Errors) > 0 {
		t.Fatalf("Query returned errors: %v", getGQL.Errors)
	}

	rawGetData, _ := json.Marshal(getGQL.Data)
	var getData struct {
		Item struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
		} `json:"item"`
	}
	if err := json.Unmarshal(rawGetData, &getData); err != nil {
		t.Fatalf("Failed to parse queried item: %v", err)
	}

	if getData.Item.Name != "CRUD Workflow Item" {
		t.Errorf("Query returned wrong name: expected %q, got %q", "CRUD Workflow Item", getData.Item.Name)
	}

	// Step 3: Update item
	t.Log("Step 3: Update item")
	updateQuery := fmt.Sprintf(
		`mutation { updateItem(id: "%s", input: {name: "Updated CRUD Item", description: "Updated description", price: 59.99}) { id name description price } }`,
		itemID,
	)
	updateResp, err := graphqlQuery(ctx, client, updateQuery)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	AssertStatusCode(t, updateResp, http.StatusOK)

	updateGQL, err := ParseGraphQLResponse(updateResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse update response: %v", err)
	}
	if len(updateGQL.Errors) > 0 {
		t.Fatalf("Update returned errors: %v", updateGQL.Errors)
	}

	// Step 4: Verify update via query
	t.Log("Step 4: Verify update via query")
	verifyQuery := fmt.Sprintf(`{ item(id: "%s") { id name description price } }`, itemID)
	verifyResp, err := graphqlQuery(ctx, client, verifyQuery)
	if err != nil {
		t.Fatalf("Verify query failed: %v", err)
	}
	AssertStatusCode(t, verifyResp, http.StatusOK)

	verifyGQL, err := ParseGraphQLResponse(verifyResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse verify response: %v", err)
	}
	if len(verifyGQL.Errors) > 0 {
		t.Fatalf("Verify query returned errors: %v", verifyGQL.Errors)
	}

	rawVerifyData, _ := json.Marshal(verifyGQL.Data)
	var verifyData struct {
		Item struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Price       float64 `json:"price"`
		} `json:"item"`
	}
	if err := json.Unmarshal(rawVerifyData, &verifyData); err != nil {
		t.Fatalf("Failed to parse verified item: %v", err)
	}

	if verifyData.Item.Name != "Updated CRUD Item" {
		t.Errorf("Expected updated name %q, got %q", "Updated CRUD Item", verifyData.Item.Name)
	}
	if verifyData.Item.Description != "Updated description" {
		t.Errorf("Expected updated description %q, got %q", "Updated description", verifyData.Item.Description)
	}
	if verifyData.Item.Price != 59.99 {
		t.Errorf("Expected updated price %f, got %f", 59.99, verifyData.Item.Price)
	}

	// Step 5: Delete item
	t.Log("Step 5: Delete item")
	deleteQuery := fmt.Sprintf(`mutation { deleteItem(id: "%s") }`, itemID)
	deleteResp, err := graphqlQuery(ctx, client, deleteQuery)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	AssertStatusCode(t, deleteResp, http.StatusOK)

	deleteGQL, err := ParseGraphQLResponse(deleteResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse delete response: %v", err)
	}
	if len(deleteGQL.Errors) > 0 {
		t.Fatalf("Delete returned errors: %v", deleteGQL.Errors)
	}

	// Step 6: Verify deletion (query returns error)
	t.Log("Step 6: Verify deletion")
	verifyDeleteQuery := fmt.Sprintf(`{ item(id: "%s") { id name } }`, itemID)
	verifyDeleteResp, err := graphqlQuery(ctx, client, verifyDeleteQuery)
	if err != nil {
		t.Fatalf("Verify delete query failed: %v", err)
	}
	AssertStatusCode(t, verifyDeleteResp, http.StatusOK)

	verifyDeleteGQL, err := ParseGraphQLResponse(verifyDeleteResp.Body)
	if err != nil {
		t.Fatalf("Failed to parse verify delete response: %v", err)
	}

	// After deletion, querying the item should return an error
	if len(verifyDeleteGQL.Errors) == 0 {
		t.Error("Expected error when querying deleted item, got none")
	}

	t.Log("GraphQL CRUD workflow completed successfully")
}

// TestFunctional_GQL_013_GraphiQLPlayground tests that GET /graphql returns the GraphiQL HTML page.
// FT-GQL-013: GraphiQL playground accessible via GET
func TestFunctional_GQL_013_GraphiQLPlayground(t *testing.T) {
	LogTestStart(t, "FT-GQL-013", "GraphiQL playground")
	defer LogTestEnd(t, "FT-GQL-013")

	ts := NewTestServer(t)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act - GET /graphql with Accept: text/html
	headers := map[string]string{
		"Accept": "text/html",
	}
	resp, err := client.Get(ctx, "/graphql", headers)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	body := string(resp.Body)
	// GraphiQL playground should return HTML content
	if !strings.Contains(body, "html") && !strings.Contains(body, "graphiql") && !strings.Contains(body, "GraphiQL") {
		t.Errorf("Expected HTML page with GraphiQL, got body prefix: %q", body[:min(200, len(body))])
	}

	t.Log("GraphiQL playground is accessible")
}

// --- Auth Tests ---

// TestFunctional_GQL_014_WithAPIKeyAuth tests GraphQL endpoint with API key authentication.
// FT-GQL-014: GraphQL works with API key auth
func TestFunctional_GQL_014_WithAPIKeyAuth(t *testing.T) {
	LogTestStart(t, "FT-GQL-014", "GraphQL with API key auth")
	defer LogTestEnd(t, "FT-GQL-014")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act - send GraphQL query with valid API key
	query := `{ items { id name } }`
	headers := APIKeyHeaders(testAPIKey)
	resp, err := graphqlQueryWithHeaders(ctx, client, query, headers)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert
	AssertStatusCode(t, resp, http.StatusOK)

	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse GraphQL response: %v", err)
	}

	if len(gqlResp.Errors) > 0 {
		t.Fatalf("Unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	t.Log("GraphQL endpoint works with API key authentication")
}

// TestFunctional_GQL_015_WithoutAuthReturns401 tests GraphQL endpoint requires auth when enabled.
// FT-GQL-015: GraphQL without auth returns 401
func TestFunctional_GQL_015_WithoutAuthReturns401(t *testing.T) {
	LogTestStart(t, "FT-GQL-015", "GraphQL without auth returns 401")
	defer LogTestEnd(t, "FT-GQL-015")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
	defer cancel()

	// Act - send GraphQL query without auth
	query := `{ items { id name } }`
	resp, err := graphqlQuery(ctx, client, query)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert - should return 401 Unauthorized
	AssertStatusCode(t, resp, http.StatusUnauthorized)

	t.Log("GraphQL endpoint correctly requires authentication")
}
