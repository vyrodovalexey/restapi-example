package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

// testKeyID is the key ID used in test JWKs.
const testKeyID = "test-key-1"

// testIssuer is the issuer URL used in tests (will be replaced by the test server URL).
const testIssuer = "http://test-issuer"

// generateTestRSAKey creates an RSA key pair for testing.
func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	return key
}

// base64URLEncode encodes bytes to base64url without padding.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// createJWKSResponse builds a JWKS JSON response from an RSA public key.
func createJWKSResponse(t *testing.T, key *rsa.PublicKey, kid string) []byte {
	t.Helper()

	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": kid,
				"alg": "RS256",
				"n":   base64URLEncode(key.N.Bytes()),
				"e":   base64URLEncode(big.NewInt(int64(key.E)).Bytes()),
			},
		},
	}

	data, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("marshalling JWKS: %v", err)
	}

	return data
}

// createDiscoveryResponse builds an OIDC discovery document JSON response.
func createDiscoveryResponse(issuer, jwksURI string) []byte {
	doc := map[string]string{
		"issuer":   issuer,
		"jwks_uri": jwksURI,
	}

	data, _ := json.Marshal(doc)

	return data
}

// signJWT creates a signed JWT token with the given header and payload using RS256.
func signJWT(t *testing.T, key *rsa.PrivateKey, header, payload map[string]any) string {
	t.Helper()

	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshalling header: %v", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshalling payload: %v", err)
	}

	headerB64 := base64URLEncode(headerJSON)
	payloadB64 := base64URLEncode(payloadJSON)
	signingInput := headerB64 + "." + payloadB64

	h := sha256.New()
	h.Write([]byte(signingInput))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, key, 0x05, digest) // crypto.SHA256 = 5
	if err != nil {
		t.Fatalf("signing JWT: %v", err)
	}

	return signingInput + "." + base64URLEncode(sig)
}

// oidcTestServer creates a test HTTP server that serves OIDC discovery and JWKS endpoints.
// Returns the server and its URL (which acts as the issuer URL).
func oidcTestServer(
	t *testing.T,
	key *rsa.PublicKey,
	kid string,
) *httptest.Server {
	t.Helper()

	var serverURL string

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(createDiscoveryResponse(serverURL, serverURL+"/jwks"))
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(createJWKSResponse(t, key, kid))
	})

	server := httptest.NewServer(mux)
	serverURL = server.URL

	t.Cleanup(server.Close)

	return server
}

// createValidToken creates a valid JWT token for testing.
func createValidToken(
	t *testing.T,
	key *rsa.PrivateKey,
	issuer string,
	subject string,
	audience any,
	expiry time.Time,
	kid string,
) string {
	t.Helper()

	header := map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": kid,
	}

	payload := map[string]any{
		"sub": subject,
		"iss": issuer,
		"aud": audience,
		"exp": float64(expiry.Unix()),
		"iat": float64(time.Now().Unix()),
	}

	return signJWT(t, key, header, payload)
}

func TestOIDCTokenVerifier_SuccessfulVerification(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	token := createValidToken(
		t, rsaKey, server.URL, "user@example.com",
		"my-api", time.Now().Add(time.Hour), testKeyID,
	)

	// Act
	claims, err := verifier.Verify(context.Background(), token)

	// Assert
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}

	if claims.Subject != "user@example.com" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user@example.com")
	}

	if claims.Issuer != server.URL {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, server.URL)
	}

	if len(claims.Audience) != 1 || claims.Audience[0] != "my-api" {
		t.Errorf("Audience = %v, want [my-api]", claims.Audience)
	}

	if claims.Claims == nil {
		t.Error("Claims map should not be nil")
	}
}

func TestOIDCTokenVerifier_MultipleAudiences(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	token := createValidToken(
		t, rsaKey, server.URL, "user@example.com",
		[]string{"api-1", "api-2"}, time.Now().Add(time.Hour), testKeyID,
	)

	// Act
	claims, err := verifier.Verify(context.Background(), token)

	// Assert
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}

	if len(claims.Audience) != 2 {
		t.Fatalf("Audience length = %d, want 2", len(claims.Audience))
	}

	if claims.Audience[0] != "api-1" || claims.Audience[1] != "api-2" {
		t.Errorf("Audience = %v, want [api-1 api-2]", claims.Audience)
	}
}

func TestOIDCTokenVerifier_ExpiredToken(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	token := createValidToken(
		t, rsaKey, server.URL, "user@example.com",
		"my-api", time.Now().Add(-time.Hour), testKeyID,
	)

	// Act
	_, err = verifier.Verify(context.Background(), token)

	// Assert
	if err == nil {
		t.Fatal("Verify() error = nil, want error for expired token")
	}

	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Errorf("Verify() error = %v, want ErrTokenExpired", err)
	}
}

func TestOIDCTokenVerifier_InvalidSignature(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	differentKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	// Sign with a different key than what the JWKS server provides.
	token := createValidToken(
		t, differentKey, server.URL, "user@example.com",
		"my-api", time.Now().Add(time.Hour), testKeyID,
	)

	// Act
	_, err = verifier.Verify(context.Background(), token)

	// Assert
	if err == nil {
		t.Fatal("Verify() error = nil, want error for invalid signature")
	}

	if !errors.Is(err, auth.ErrSignatureInvalid) {
		t.Errorf("Verify() error = %v, want ErrSignatureInvalid", err)
	}
}

func TestOIDCTokenVerifier_WrongIssuer(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	// Create token with a different issuer.
	token := createValidToken(
		t, rsaKey, "http://wrong-issuer", "user@example.com",
		"my-api", time.Now().Add(time.Hour), testKeyID,
	)

	// Act
	_, err = verifier.Verify(context.Background(), token)

	// Assert
	if err == nil {
		t.Fatal("Verify() error = nil, want error for wrong issuer")
	}

	if !errors.Is(err, auth.ErrIssuerMismatch) {
		t.Errorf("Verify() error = %v, want ErrIssuerMismatch", err)
	}
}

func TestOIDCTokenVerifier_WrongAudience(t *testing.T) {
	t.Parallel()

	// Arrange: verifier does not check audience itself; that is done by
	// OIDCAuthenticator. But we verify the audience is correctly extracted.
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	// Use OIDCAuthenticator with audience check.
	authenticator := auth.NewOIDCAuthenticator(verifier, "expected-api")

	token := createValidToken(
		t, rsaKey, server.URL, "user@example.com",
		"wrong-api", time.Now().Add(time.Hour), testKeyID,
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	// Act
	_, err = authenticator.Authenticate(req)

	// Assert
	if err == nil {
		t.Fatal("Authenticate() error = nil, want error for wrong audience")
	}

	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("Authenticate() error = %v, want ErrInvalidToken", err)
	}
}

func TestOIDCTokenVerifier_JWKSFetchFailure(t *testing.T) {
	t.Parallel()

	// Arrange: create a server that returns errors for JWKS.
	mux := http.NewServeMux()

	var serverURL string

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(createDiscoveryResponse(serverURL, serverURL+"/jwks"))
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	})

	server := httptest.NewServer(mux)
	serverURL = server.URL

	defer server.Close()

	// Act
	_, err := auth.NewOIDCTokenVerifier(server.URL)

	// Assert
	if err == nil {
		t.Fatal("NewOIDCTokenVerifier() error = nil, want error for JWKS fetch failure")
	}

	if !strings.Contains(err.Error(), "JWKS") {
		t.Errorf("error = %v, want error containing 'JWKS'", err)
	}
}

func TestOIDCTokenVerifier_MalformedToken(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "single part",
			token: "header-only",
		},
		{
			name:  "two parts",
			token: "header.payload",
		},
		{
			name:  "invalid base64 header",
			token: "!!!.payload.signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := verifier.Verify(context.Background(), tt.token)
			if err == nil {
				t.Error("Verify() error = nil, want error for malformed token")
			}
		})
	}
}

func TestOIDCTokenVerifier_KeyRotation(t *testing.T) {
	t.Parallel()

	// Arrange: create a server that can rotate keys.
	rsaKey1 := generateTestRSAKey(t)
	rsaKey2 := generateTestRSAKey(t)

	var currentKeyIndex atomic.Int32

	var serverURL string

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(createDiscoveryResponse(serverURL, serverURL+"/jwks"))
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		idx := currentKeyIndex.Load()
		if idx == 0 {
			w.Write(createJWKSResponse(t, &rsaKey1.PublicKey, "key-1"))
		} else {
			// After rotation, serve both keys.
			jwks := map[string]any{
				"keys": []map[string]any{
					{
						"kty": "RSA",
						"use": "sig",
						"kid": "key-1",
						"alg": "RS256",
						"n":   base64URLEncode(rsaKey1.PublicKey.N.Bytes()),
						"e":   base64URLEncode(big.NewInt(int64(rsaKey1.PublicKey.E)).Bytes()),
					},
					{
						"kty": "RSA",
						"use": "sig",
						"kid": "key-2",
						"alg": "RS256",
						"n":   base64URLEncode(rsaKey2.PublicKey.N.Bytes()),
						"e":   base64URLEncode(big.NewInt(int64(rsaKey2.PublicKey.E)).Bytes()),
					},
				},
			}
			data, _ := json.Marshal(jwks)
			w.Write(data)
		}
	})

	server := httptest.NewServer(mux)
	serverURL = server.URL

	defer server.Close()

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	// Verify with key-1 works.
	token1 := createValidToken(
		t, rsaKey1, server.URL, "user1@example.com",
		"my-api", time.Now().Add(time.Hour), "key-1",
	)

	claims, err := verifier.Verify(context.Background(), token1)
	if err != nil {
		t.Fatalf("Verify() with key-1 error = %v, want nil", err)
	}

	if claims.Subject != "user1@example.com" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user1@example.com")
	}

	// Rotate keys: now serve key-2 as well.
	currentKeyIndex.Store(1)

	// Verify with key-2 works (should trigger a JWKS refresh since key-2 is unknown).
	token2 := createValidToken(
		t, rsaKey2, server.URL, "user2@example.com",
		"my-api", time.Now().Add(time.Hour), "key-2",
	)

	claims, err = verifier.Verify(context.Background(), token2)
	if err != nil {
		t.Fatalf("Verify() with key-2 after rotation error = %v, want nil", err)
	}

	if claims.Subject != "user2@example.com" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user2@example.com")
	}
}

func TestOIDCTokenVerifier_UnknownKeyID(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	// Create token with unknown key ID.
	token := createValidToken(
		t, rsaKey, server.URL, "user@example.com",
		"my-api", time.Now().Add(time.Hour), "unknown-key-id",
	)

	// Act
	_, err = verifier.Verify(context.Background(), token)

	// Assert
	if err == nil {
		t.Fatal("Verify() error = nil, want error for unknown key ID")
	}

	if !errors.Is(err, auth.ErrKeyNotFound) {
		t.Errorf("Verify() error = %v, want ErrKeyNotFound", err)
	}
}

func TestOIDCTokenVerifier_DiscoveryFetchFailure(t *testing.T) {
	t.Parallel()

	// Arrange: server that returns error for discovery.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Act
	_, err := auth.NewOIDCTokenVerifier(server.URL)

	// Assert
	if err == nil {
		t.Fatal("NewOIDCTokenVerifier() error = nil, want error for discovery failure")
	}
}

func TestOIDCTokenVerifier_UnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	// Create token with unsupported algorithm.
	header := map[string]any{
		"alg": "ES256",
		"typ": "JWT",
		"kid": testKeyID,
	}
	payload := map[string]any{
		"sub": "user@example.com",
		"iss": server.URL,
		"aud": "my-api",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
		"iat": float64(time.Now().Unix()),
	}

	token := signJWT(t, rsaKey, header, payload)

	// Act
	_, err = verifier.Verify(context.Background(), token)

	// Assert
	if err == nil {
		t.Fatal("Verify() error = nil, want error for unsupported algorithm")
	}

	if !errors.Is(err, auth.ErrUnsupportedAlgo) {
		t.Errorf("Verify() error = %v, want ErrUnsupportedAlgo", err)
	}
}

func TestOIDCTokenVerifier_ClaimsExtraction(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	header := map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": testKeyID,
	}
	payload := map[string]any{
		"sub":   "user@example.com",
		"iss":   server.URL,
		"aud":   "my-api",
		"exp":   float64(time.Now().Add(time.Hour).Unix()),
		"iat":   float64(time.Now().Unix()),
		"role":  "admin",
		"scope": "read write",
	}

	token := signJWT(t, rsaKey, header, payload)

	// Act
	claims, err := verifier.Verify(context.Background(), token)

	// Assert
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}

	if claims.Claims["role"] != "admin" {
		t.Errorf("Claims[role] = %v, want %q", claims.Claims["role"], "admin")
	}

	if claims.Claims["scope"] != "read write" {
		t.Errorf("Claims[scope] = %v, want %q", claims.Claims["scope"], "read write")
	}
}

func TestOIDCTokenVerifier_FullIntegrationWithAuthenticator(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	authenticator := auth.NewOIDCAuthenticator(verifier, "my-api")

	token := createValidToken(
		t, rsaKey, server.URL, "user@example.com",
		"my-api", time.Now().Add(time.Hour), testKeyID,
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	// Act
	info, err := authenticator.Authenticate(req)

	// Assert
	if err != nil {
		t.Fatalf("Authenticate() error = %v, want nil", err)
	}

	if info.Method != auth.AuthMethodOIDC {
		t.Errorf("Method = %q, want %q", info.Method, auth.AuthMethodOIDC)
	}

	if info.Subject != "user@example.com" {
		t.Errorf("Subject = %q, want %q", info.Subject, "user@example.com")
	}
}

func TestOIDCTokenVerifier_DiscoveryMissingJWKSURI(t *testing.T) {
	t.Parallel()

	// Arrange: discovery document without jwks_uri.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"issuer": "http://test"}`)

			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Act
	_, err := auth.NewOIDCTokenVerifier(server.URL)

	// Assert
	if err == nil {
		t.Fatal("NewOIDCTokenVerifier() error = nil, want error for missing jwks_uri")
	}

	if !strings.Contains(err.Error(), "jwks_uri") {
		t.Errorf("error = %v, want error containing 'jwks_uri'", err)
	}
}

func TestOIDCTokenVerifier_ExpiryBoundary(t *testing.T) {
	t.Parallel()

	// Arrange
	rsaKey := generateTestRSAKey(t)
	server := oidcTestServer(t, &rsaKey.PublicKey, testKeyID)

	verifier, err := auth.NewOIDCTokenVerifier(server.URL)
	if err != nil {
		t.Fatalf("creating verifier: %v", err)
	}
	defer verifier.Stop()

	// Token that expired 1 second ago.
	token := createValidToken(
		t, rsaKey, server.URL, "user@example.com",
		"my-api", time.Now().Add(-1*time.Second), testKeyID,
	)

	// Act
	_, err = verifier.Verify(context.Background(), token)

	// Assert
	if err == nil {
		t.Fatal("Verify() error = nil, want error for just-expired token")
	}

	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Errorf("Verify() error = %v, want ErrTokenExpired", err)
	}
}
