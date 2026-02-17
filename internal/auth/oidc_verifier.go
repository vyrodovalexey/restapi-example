package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// OIDC verifier errors.
var (
	ErrJWKSFetch        = errors.New("failed to fetch JWKS")
	ErrTokenMalformed   = errors.New("malformed JWT token")
	ErrTokenExpired     = errors.New("token has expired")
	ErrIssuerMismatch   = errors.New("token issuer mismatch")
	ErrKeyNotFound      = errors.New("signing key not found in JWKS")
	ErrUnsupportedAlgo  = errors.New("unsupported signing algorithm")
	ErrSignatureInvalid = errors.New("token signature is invalid")
)

// jwksRefreshInterval defines how often the JWKS keys are refreshed.
const jwksRefreshInterval = 5 * time.Minute

// maxJWKSRetries is the maximum number of retry attempts for JWKS fetch.
const maxJWKSRetries = 3

// initialRetryDelay is the base delay for exponential backoff.
const initialRetryDelay = 500 * time.Millisecond

// httpClientTimeout is the timeout for HTTP requests to the OIDC provider.
const httpClientTimeout = 10 * time.Second

// oidcDiscoveryDocument represents the OpenID Connect discovery document.
type oidcDiscoveryDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

// jwksDocument represents a JSON Web Key Set document.
type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

// jwkKey represents a single JSON Web Key.
type jwkKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwtHeader represents the header portion of a JWT.
type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// jwtPayload represents the payload portion of a JWT with standard claims.
type jwtPayload struct {
	Sub string   `json:"sub"`
	Iss string   `json:"iss"`
	Aud audience `json:"aud"`
	Exp float64  `json:"exp"`
	Iat float64  `json:"iat"`
}

// audience handles both string and []string JSON representations of the "aud" claim.
type audience []string

// UnmarshalJSON implements custom unmarshalling for the audience claim,
// which can be either a single string or an array of strings per RFC 7519.
func (a *audience) UnmarshalJSON(data []byte) error {
	// Try single string first.
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*a = []string{single}
		return nil
	}

	// Try array of strings.
	var multi []string
	if err := json.Unmarshal(data, &multi); err != nil {
		return fmt.Errorf("audience must be a string or array of strings: %w", err)
	}

	*a = multi

	return nil
}

// OIDCTokenVerifier implements the TokenVerifier interface by validating
// JWT tokens against a JWKS endpoint discovered from an OIDC provider.
type OIDCTokenVerifier struct {
	issuerURL string
	jwksURI   string
	client    *http.Client

	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey // kid -> public key

	stopRefresh chan struct{}
}

// NewOIDCTokenVerifier creates a new OIDC token verifier that fetches
// the discovery document and JWKS from the given issuer URL.
// It starts a background goroutine to periodically refresh the JWKS keys.
func NewOIDCTokenVerifier(issuerURL string) (*OIDCTokenVerifier, error) {
	client := &http.Client{Timeout: httpClientTimeout}

	// Fetch the OIDC discovery document to obtain the JWKS URI.
	discoveryURL := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"

	disc, err := fetchDiscoveryDocument(client, discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("fetching OIDC discovery document: %w", err)
	}

	v := &OIDCTokenVerifier{
		issuerURL:   issuerURL,
		jwksURI:     disc.JWKSURI,
		client:      client,
		keys:        make(map[string]*rsa.PublicKey),
		stopRefresh: make(chan struct{}),
	}

	// Perform initial JWKS fetch with retries.
	if err := v.refreshKeys(); err != nil {
		return nil, fmt.Errorf("initial JWKS fetch: %w", err)
	}

	// Start background JWKS refresh goroutine.
	go v.backgroundRefresh()

	return v, nil
}

// Stop terminates the background JWKS refresh goroutine.
func (v *OIDCTokenVerifier) Stop() {
	close(v.stopRefresh)
}

// Verify validates the given raw JWT token string and returns the extracted claims.
// It checks the token signature, expiry, and issuer.
func (v *OIDCTokenVerifier) Verify(ctx context.Context, rawToken string) (*TokenClaims, error) {
	header, payload, err := v.parseAndVerifySignature(ctx, rawToken)
	if err != nil {
		return nil, err
	}

	// Validate issuer.
	if payload.Iss != v.issuerURL {
		return nil, fmt.Errorf(
			"%w: expected %q, got %q",
			ErrIssuerMismatch, v.issuerURL, payload.Iss,
		)
	}

	// Validate expiry.
	expiry := time.Unix(int64(payload.Exp), 0)
	if time.Now().After(expiry) {
		return nil, fmt.Errorf("%w: expired at %v", ErrTokenExpired, expiry)
	}

	// Build all claims map from the raw payload for extensibility.
	allClaims, err := extractAllClaims(rawToken)
	if err != nil {
		return nil, fmt.Errorf("extracting claims: %w", err)
	}

	_ = header // header was used for signature verification

	return &TokenClaims{
		Subject:  payload.Sub,
		Audience: []string(payload.Aud),
		Issuer:   payload.Iss,
		Expiry:   expiry,
		Claims:   allClaims,
	}, nil
}

// parseAndVerifySignature splits the JWT, verifies the signature, and returns
// the parsed header and payload.
func (v *OIDCTokenVerifier) parseAndVerifySignature(
	_ context.Context,
	rawToken string,
) (*jwtHeader, *jwtPayload, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, nil, fmt.Errorf("%w: expected 3 parts, got %d", ErrTokenMalformed, len(parts))
	}

	// Decode and parse header.
	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decoding header: %w", ErrTokenMalformed, err)
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, nil, fmt.Errorf("%w: parsing header: %w", ErrTokenMalformed, err)
	}

	// Validate signing algorithm.
	hashFunc, err := rsaHashForAlgorithm(header.Alg)
	if err != nil {
		return nil, nil, err
	}

	// Look up the signing key.
	key, err := v.getKey(header.Kid)
	if err != nil {
		return nil, nil, err
	}

	// Verify signature.
	signingInput := parts[0] + "." + parts[1]
	signatureBytes, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decoding signature: %w", ErrTokenMalformed, err)
	}

	if err := verifyRSASignature(key, hashFunc, []byte(signingInput), signatureBytes); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrSignatureInvalid, err)
	}

	// Decode and parse payload.
	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decoding payload: %w", ErrTokenMalformed, err)
	}

	var payload jwtPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, nil, fmt.Errorf("%w: parsing payload: %w", ErrTokenMalformed, err)
	}

	return &header, &payload, nil
}

// getKey retrieves the RSA public key for the given key ID from the cached JWKS.
func (v *OIDCTokenVerifier) getKey(kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	v.mu.RUnlock()

	if ok {
		return key, nil
	}

	// Key not found; attempt a refresh in case keys were rotated.
	if err := v.refreshKeys(); err != nil {
		return nil, fmt.Errorf("%w: refresh failed: %w", ErrKeyNotFound, err)
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: kid=%q", ErrKeyNotFound, kid)
	}

	return key, nil
}

// refreshKeys fetches the JWKS document and updates the cached keys.
// Uses exponential backoff for retries.
func (v *OIDCTokenVerifier) refreshKeys() error {
	var lastErr error
	delay := initialRetryDelay

	for attempt := range maxJWKSRetries {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
		}

		keys, err := fetchJWKS(v.client, v.jwksURI)
		if err != nil {
			lastErr = err
			continue
		}

		v.mu.Lock()
		v.keys = keys
		v.mu.Unlock()

		return nil
	}

	return fmt.Errorf("%w: after %d attempts: %w", ErrJWKSFetch, maxJWKSRetries, lastErr)
}

// backgroundRefresh periodically refreshes the JWKS keys.
func (v *OIDCTokenVerifier) backgroundRefresh() {
	ticker := time.NewTicker(jwksRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-v.stopRefresh:
			return
		case <-ticker.C:
			// Best-effort refresh; errors are silently ignored because
			// the previously cached keys remain valid until the next
			// successful refresh.
			_ = v.refreshKeys()
		}
	}
}

// fetchDiscoveryDocument retrieves the OIDC discovery document from the given URL.
func fetchDiscoveryDocument(client *http.Client, url string) (*oidcDiscoveryDocument, error) {
	resp, err := client.Get(url) //nolint:noctx // discovery fetch is a one-time startup operation
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var doc oidcDiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decoding discovery document: %w", err)
	}

	if doc.JWKSURI == "" {
		return nil, fmt.Errorf("discovery document missing jwks_uri")
	}

	return &doc, nil
}

// fetchJWKS retrieves the JWKS document and parses RSA public keys.
func fetchJWKS(client *http.Client, jwksURI string) (map[string]*rsa.PublicKey, error) {
	resp, err := client.Get(jwksURI) //nolint:noctx // JWKS fetch uses short-lived HTTP client with timeout
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var doc jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decoding JWKS document: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))

	for _, jwk := range doc.Keys {
		if jwk.Kty != "RSA" {
			continue
		}

		if jwk.Use != "" && jwk.Use != "sig" {
			continue
		}

		pubKey, err := parseRSAPublicKey(jwk)
		if err != nil {
			continue // Skip keys that cannot be parsed.
		}

		keys[jwk.Kid] = pubKey
	}

	return keys, nil
}

// parseRSAPublicKey constructs an RSA public key from a JWK.
func parseRSAPublicKey(jwk jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64URLDecode(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("decoding modulus: %w", err)
	}

	eBytes, err := base64URLDecode(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("decoding exponent: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// base64URLDecode decodes a base64url-encoded string (without padding).
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if necessary.
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}

	return base64.URLEncoding.DecodeString(s)
}

// extractAllClaims decodes the payload section of a JWT and returns all claims as a map.
func extractAllClaims(rawToken string) (map[string]any, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding payload: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("parsing claims: %w", err)
	}

	return claims, nil
}

// rsaHashForAlgorithm returns the hash function and crypto.Hash identifier
// for the given RSA signing algorithm.
func rsaHashForAlgorithm(alg string) (crypto.Hash, error) {
	switch alg {
	case "RS256":
		return crypto.SHA256, nil
	case "RS384":
		return crypto.SHA384, nil
	case "RS512":
		return crypto.SHA512, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnsupportedAlgo, alg)
	}
}

// newHashFunc creates a new hash.Hash for the given crypto.Hash.
func newHashFunc(h crypto.Hash) hash.Hash {
	switch h {
	case crypto.SHA256:
		return sha256.New()
	case crypto.SHA384:
		return sha512.New384()
	case crypto.SHA512:
		return sha512.New()
	default:
		return sha256.New()
	}
}

// verifyRSASignature verifies an RSA PKCS#1 v1.5 signature.
func verifyRSASignature(
	key *rsa.PublicKey,
	hashAlg crypto.Hash,
	signingInput []byte,
	signature []byte,
) error {
	h := newHashFunc(hashAlg)
	h.Write(signingInput)
	digest := h.Sum(nil)

	return rsa.VerifyPKCS1v15(key, hashAlg, digest, signature)
}
