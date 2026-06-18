//go:build functional

package functional

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestFunctional_AUTH_OIDC_GraphQL verifies the GraphQL endpoint is reachable
// with a valid OIDC bearer token and rejected without one. Extends the OIDC
// auth coverage to the GraphQL endpoint group.
func TestFunctional_AUTH_OIDC_GraphQL(t *testing.T) {
	LogTestStart(t, "FT-AUTH-OIDC-GQL", "OIDC auth on GraphQL endpoint")
	defer LogTestEnd(t, "FT-AUTH-OIDC-GQL")

	ts := NewTestServerWithOIDCAuth(t, testOIDCAudience)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	token := CreateMockToken(
		"test-user", testOIDCIssuer,
		[]string{testOIDCAudience}, time.Now().Add(time.Hour),
	)

	// With valid token: 200, no GraphQL errors.
	resp, err := graphqlQueryWithHeaders(
		ctx, client, `{ items { id name } }`, BearerTokenHeaders(token),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusOK)
	gqlResp, err := ParseGraphQLResponse(resp.Body)
	if err != nil {
		t.Fatalf("failed to parse GraphQL response: %v", err)
	}
	if len(gqlResp.Errors) > 0 {
		t.Errorf("unexpected GraphQL errors: %v", gqlResp.Errors)
	}

	// Without token: 401.
	resp, err = graphqlQuery(ctx, client, `{ items { id name } }`)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusUnauthorized)
}

// TestFunctional_AUTH_MULTI_GraphQL verifies the GraphQL endpoint works under
// multi-auth mode via either API key or basic auth, and rejects no-auth.
func TestFunctional_AUTH_MULTI_GraphQL(t *testing.T) {
	LogTestStart(t, "FT-AUTH-MULTI-GQL", "multi-auth on GraphQL endpoint")
	defer LogTestEnd(t, "FT-AUTH-MULTI-GQL")

	usersConfig := generateTestBasicAuthConfig(t)
	ts := NewTestServerWithMultiAuth(t, testAPIKeyConfig, usersConfig)
	ts.Start()
	defer ts.Stop()

	client := NewHTTPClient(t, ts.BaseURL)
	ctx, cancel := context.WithTimeout(
		context.Background(), DefaultRequestTimeout,
	)
	defer cancel()

	// API key path.
	resp, err := graphqlQueryWithHeaders(
		ctx, client, `{ items { id name } }`, APIKeyHeaders(testAPIKey),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusOK)

	// No-auth path: 401.
	resp, err = graphqlQuery(ctx, client, `{ items { id name } }`)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	AssertStatusCode(t, resp, http.StatusUnauthorized)
}

// TestFunctional_AUTH_WS_UpgradeSkipsAuth documents and verifies the current
// behaviour that WebSocket upgrade requests bypass the auth middleware (the
// Auth middleware explicitly skips Upgrade: websocket). The upgrade should
// succeed even on an auth-enabled server.
func TestFunctional_AUTH_WS_UpgradeSkipsAuth(t *testing.T) {
	LogTestStart(t, "FT-AUTH-WS", "WebSocket upgrade bypasses auth")
	defer LogTestEnd(t, "FT-AUTH-WS")

	ts := NewTestServerWithAPIKeyAuth(t, testAPIKeyConfig)
	ts.Start()
	defer ts.Stop()

	// No auth header on the WS handshake; upgrade should still succeed.
	wsClient, err := NewWebSocketClient(t, ts.WSURL+"/ws")
	if err != nil {
		t.Fatalf("WebSocket upgrade failed on auth-enabled server: %v", err)
	}
	defer wsClient.Close()

	msg, err := wsClient.ReadMessage(3 * time.Second)
	if err != nil {
		t.Fatalf("failed to read WS message: %v", err)
	}
	if msg.Type != "random_value" {
		t.Errorf("expected message type 'random_value', got %q", msg.Type)
	}
}
