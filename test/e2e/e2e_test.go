//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestE2E_FullCRUDWorkflow exercises the complete user journey:
// create → read → update → verify update → delete → verify delete.
func TestE2E_FullCRUDWorkflow(t *testing.T) {
	skipIfServerUnavailable(t)

	base := e2eServerURL()
	client := newHTTPClient()
	headers := buildAuthHeaders(t)

	// Step 1: Create
	t.Log("Step 1: Create item")
	created := createItem(t, client, base, headers, createItemRequest{
		Name:        "E2E Workflow Item",
		Description: "Created during E2E test",
		Price:       99.99,
	})

	if created.ID == "" {
		t.Fatal("Created item has empty ID")
	}
	t.Logf("Created item ID=%s", created.ID)

	itemURL := fmt.Sprintf(
		"%s/api/v1/items/%s", base, created.ID,
	)

	// Step 2: Read
	t.Log("Step 2: Read item")
	status, body := doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	if status != http.StatusOK {
		t.Fatalf("Read: expected 200, got %d. Body: %s",
			status, body)
	}

	var readResp apiResponse
	if err := json.Unmarshal(body, &readResp); err != nil {
		t.Fatalf("Failed to parse read response: %v", err)
	}

	var readItem itemResponse
	if err := json.Unmarshal(readResp.Data, &readItem); err != nil {
		t.Fatalf("Failed to parse read item: %v", err)
	}

	if readItem.Name != "E2E Workflow Item" {
		t.Errorf(
			"Read: expected name 'E2E Workflow Item', got %q",
			readItem.Name,
		)
	}

	// Step 3: Update
	t.Log("Step 3: Update item")
	updatePayload, _ := json.Marshal(updateItemRequest{
		Name:        "E2E Updated Item",
		Description: "Updated during E2E test",
		Price:       149.99,
	})

	status, body = doRequest(
		t, client, http.MethodPut, itemURL,
		bytes.NewReader(updatePayload), headers,
	)

	if status != http.StatusOK {
		t.Fatalf("Update: expected 200, got %d. Body: %s",
			status, body)
	}

	// Step 4: Verify update
	t.Log("Step 4: Verify update")
	status, body = doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	if status != http.StatusOK {
		t.Fatalf("Verify update: expected 200, got %d", status)
	}

	var verifyResp apiResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		t.Fatalf("Failed to parse verify response: %v", err)
	}

	var verifyItem itemResponse
	if err := json.Unmarshal(verifyResp.Data, &verifyItem); err != nil {
		t.Fatalf("Failed to parse verify item: %v", err)
	}

	if verifyItem.Name != "E2E Updated Item" {
		t.Errorf(
			"Verify: expected 'E2E Updated Item', got %q",
			verifyItem.Name,
		)
	}
	if verifyItem.Price != 149.99 {
		t.Errorf(
			"Verify: expected price 149.99, got %f",
			verifyItem.Price,
		)
	}

	// Step 5: Delete
	t.Log("Step 5: Delete item")
	status, body = doRequest(
		t, client, http.MethodDelete, itemURL, nil, headers,
	)

	if status != http.StatusNoContent {
		t.Fatalf("Delete: expected 204, got %d. Body: %s",
			status, body)
	}

	// Step 6: Verify delete
	t.Log("Step 6: Verify delete")
	status, _ = doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	if status != http.StatusNotFound {
		t.Errorf("Verify delete: expected 404, got %d", status)
	}

	t.Log("Full CRUD workflow completed successfully")
}

// TestE2E_APIKeyWorkflow tests the complete API key authentication
// workflow: authenticate with API key → perform CRUD operations.
func TestE2E_APIKeyWorkflow(t *testing.T) {
	apiKey := os.Getenv(EnvAPIKey)
	if apiKey == "" {
		t.Skip("INTEGRATION_API_KEY not set, skipping")
	}

	skipIfServerUnavailable(t)

	base := e2eServerURL()
	client := newHTTPClient()
	headers := map[string]string{
		"X-API-Key":    apiKey,
		"Content-Type": "application/json",
	}

	// Create
	created := createItem(t, client, base, headers, createItemRequest{
		Name:  "API Key Workflow Item",
		Price: 25.00,
	})
	t.Logf("Created item with API key auth: ID=%s", created.ID)

	// Read
	itemURL := fmt.Sprintf(
		"%s/api/v1/items/%s", base, created.ID,
	)
	status, body := doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	if status != http.StatusOK {
		t.Errorf("Read with API key: expected 200, got %d", status)
	}

	// Cleanup
	deleteItem(t, client, base, created.ID, headers)

	// Verify cleanup
	status, _ = doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)
	if status != http.StatusNotFound {
		t.Errorf(
			"Cleanup verify: expected 404, got %d. Body: %s",
			status, body,
		)
	}

	t.Log("API key workflow completed successfully")
}

// TestE2E_BasicAuthWorkflow tests the complete Basic auth workflow:
// authenticate with credentials → perform CRUD operations.
func TestE2E_BasicAuthWorkflow(t *testing.T) {
	user := os.Getenv(EnvBasicUser)
	pass := os.Getenv(EnvBasicPass)
	if user == "" || pass == "" {
		t.Skip("INTEGRATION_BASIC_USER/PASS not set, skipping")
	}

	skipIfServerUnavailable(t)

	base := e2eServerURL()
	client := newHTTPClient()
	headers := buildAuthHeaders(t)

	// Create
	created := createItem(t, client, base, headers, createItemRequest{
		Name:  "Basic Auth Workflow Item",
		Price: 35.00,
	})
	t.Logf("Created item with basic auth: ID=%s", created.ID)

	// Read
	itemURL := fmt.Sprintf(
		"%s/api/v1/items/%s", base, created.ID,
	)
	status, _ := doRequest(
		t, client, http.MethodGet, itemURL, nil, headers,
	)

	if status != http.StatusOK {
		t.Errorf(
			"Read with basic auth: expected 200, got %d", status,
		)
	}

	// Cleanup
	deleteItem(t, client, base, created.ID, headers)

	t.Log("Basic auth workflow completed successfully")
}

// TestE2E_PublicEndpointsAlwaysAccessible verifies that health,
// ready, and metrics endpoints are accessible without authentication.
func TestE2E_PublicEndpointsAlwaysAccessible(t *testing.T) {
	t.Parallel()
	skipIfServerUnavailable(t)

	base := e2eServerURL()
	client := newHTTPClient()

	// No auth headers — these should always work.
	endpoints := []struct {
		path           string
		expectedStatus int
	}{
		{"/health", http.StatusOK},
		{"/ready", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			status, body := doRequest(
				t, client, http.MethodGet,
				base+ep.path, nil, nil,
			)

			if status != ep.expectedStatus {
				t.Errorf(
					"Expected %d for %s, got %d. Body: %s",
					ep.expectedStatus, ep.path, status, body,
				)
			}
		})
	}

	// Metrics may be disabled, so we accept 200 or 404.
	t.Run("/metrics", func(t *testing.T) {
		status, body := doRequest(
			t, client, http.MethodGet,
			base+"/metrics", nil, nil,
		)

		if status != http.StatusOK &&
			status != http.StatusNotFound {
			t.Errorf(
				"Expected 200 or 404 for /metrics, got %d. Body: %s",
				status, body,
			)
		}
	})

	t.Log("Public endpoints accessibility verified")
}

// TestE2E_UnauthorizedAccessDenied verifies that all protected
// endpoints return 401 when no credentials are provided.
func TestE2E_UnauthorizedAccessDenied(t *testing.T) {
	t.Parallel()

	apiKey := os.Getenv(EnvAPIKey)
	user := os.Getenv(EnvBasicUser)
	if apiKey == "" && user == "" {
		t.Skip("No auth configured, skipping unauthorized test")
	}

	skipIfServerUnavailable(t)

	base := e2eServerURL()
	client := newHTTPClient()

	protectedEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/items"},
		{http.MethodPost, "/api/v1/items"},
		{http.MethodGet, "/api/v1/items/nonexistent"},
		{http.MethodPut, "/api/v1/items/nonexistent"},
		{http.MethodDelete, "/api/v1/items/nonexistent"},
	}

	for _, ep := range protectedEndpoints {
		name := fmt.Sprintf("%s_%s", ep.method, ep.path)
		t.Run(name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if ep.method == http.MethodPost ||
				ep.method == http.MethodPut {
				payload, _ := json.Marshal(map[string]any{
					"name":  "test",
					"price": 1.0,
				})
				bodyReader = bytes.NewReader(payload)
			}

			var reqHeaders map[string]string
			if bodyReader != nil {
				reqHeaders = map[string]string{
					"Content-Type": "application/json",
				}
			}

			var bodyIO *bytes.Reader
			if bodyReader != nil {
				bodyIO = bodyReader
			}

			status, body := doRequest(
				t, client, ep.method,
				base+ep.path, bodyIO, reqHeaders,
			)

			if status != http.StatusUnauthorized {
				t.Errorf(
					"Expected 401 for %s %s, got %d. Body: %s",
					ep.method, ep.path, status, body,
				)
			}
		})
	}

	t.Log("Unauthorized access denial verified")
}

// TestE2E_ConcurrentRequests verifies that the server handles 10
// concurrent authenticated requests correctly.
func TestE2E_ConcurrentRequests(t *testing.T) {
	skipIfServerUnavailable(t)

	base := e2eServerURL()
	client := newHTTPClient()
	headers := buildAuthHeaders(t)

	const numConcurrent = 10
	var wg sync.WaitGroup

	type result struct {
		status int
		itemID string
		err    error
	}

	results := make(chan result, numConcurrent)

	for i := range numConcurrent {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			item := createItemRequest{
				Name: fmt.Sprintf(
					"Concurrent Item %d %s",
					idx,
					time.Now().Format(time.RFC3339Nano),
				),
				Price: float64(idx) * 10.0,
			}

			payload, _ := json.Marshal(item)
			status, body := doRequest(
				t, client, http.MethodPost,
				base+"/api/v1/items",
				bytes.NewReader(payload), headers,
			)

			r := result{status: status}
			if status == http.StatusCreated {
				var resp apiResponse
				if err := json.Unmarshal(body, &resp); err == nil {
					var created itemResponse
					if err := json.Unmarshal(
						resp.Data, &created,
					); err == nil {
						r.itemID = created.ID
					}
				}
			}
			results <- r
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	var createdIDs []string

	for r := range results {
		if r.status == http.StatusCreated {
			successCount++
			if r.itemID != "" {
				createdIDs = append(createdIDs, r.itemID)
			}
		} else {
			t.Errorf(
				"Concurrent request: expected 201, got %d",
				r.status,
			)
		}
	}

	if successCount != numConcurrent {
		t.Errorf(
			"Expected %d successful creates, got %d",
			numConcurrent, successCount,
		)
	}

	// Cleanup created items.
	for _, id := range createdIDs {
		deleteItem(t, client, base, id, headers)
	}

	t.Logf(
		"Concurrent requests test passed: %d/%d succeeded",
		successCount, numConcurrent,
	)
}

// TestE2E_GracefulDegradation verifies that the server handles
// invalid authentication gracefully without crashing.
func TestE2E_GracefulDegradation(t *testing.T) {
	t.Parallel()
	skipIfServerUnavailable(t)

	base := e2eServerURL()
	client := newHTTPClient()

	testCases := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "empty_api_key",
			headers: map[string]string{
				"X-API-Key": "",
			},
		},
		{
			name: "invalid_api_key",
			headers: map[string]string{
				"X-API-Key": "completely-invalid-key",
			},
		},
		{
			name: "malformed_bearer_token",
			headers: map[string]string{
				"Authorization": "Bearer not.a.valid.jwt",
			},
		},
		{
			name: "invalid_basic_auth",
			headers: map[string]string{
				"Authorization": "Basic " +
					"bm90OnZhbGlk", // not:valid
			},
		},
		{
			name: "garbage_auth_header",
			headers: map[string]string{
				"Authorization": "GarbageScheme xyz123",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := doRequest(
				t, client, http.MethodGet,
				base+"/api/v1/items", nil, tc.headers,
			)

			// Server should respond (not crash). We accept 401 or
			// 200 (if no auth is configured on the server).
			if status == 0 {
				t.Error("Server did not respond")
			}

			// If auth is enabled, we expect 401.
			apiKey := os.Getenv(EnvAPIKey)
			user := os.Getenv(EnvBasicUser)
			if apiKey != "" || user != "" {
				if status != http.StatusUnauthorized {
					t.Errorf(
						"Expected 401, got %d. Body: %s",
						status, body,
					)
				}
			}

			// Verify server is still healthy after bad request.
			healthStatus, _ := doRequest(
				t, client, http.MethodGet,
				base+"/health", nil, nil,
			)
			if healthStatus != http.StatusOK {
				t.Errorf(
					"Server unhealthy after bad auth: status=%d",
					healthStatus,
				)
			}
		})
	}

	// Verify server is still healthy after all bad requests.
	status, _ := doRequest(
		t, client, http.MethodGet,
		base+"/health", nil, nil,
	)
	if status != http.StatusOK {
		t.Error("Server unhealthy after graceful degradation tests")
	}

	// Verify metrics endpoint still works (if enabled).
	metricsStatus, metricsBody := doRequest(
		t, client, http.MethodGet,
		base+"/metrics", nil, nil,
	)
	if metricsStatus == http.StatusOK {
		if !strings.Contains(string(metricsBody), "# HELP") {
			t.Error("Metrics endpoint returned unexpected format")
		}
	}

	t.Log("Graceful degradation test passed")
}
