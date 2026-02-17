package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TokenVerifier verifies JWT/OIDC tokens.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*TokenClaims, error)
}

// TokenClaims holds the claims from a verified token.
type TokenClaims struct {
	Subject  string
	Audience []string
	Issuer   string
	Expiry   time.Time
	Claims   map[string]any
}

// OIDCAuthenticator authenticates requests using OIDC/JWT bearer tokens.
type OIDCAuthenticator struct {
	verifier TokenVerifier
	audience string
}

// NewOIDCAuthenticator creates a new OIDC authenticator with the given
// token verifier and expected audience.
func NewOIDCAuthenticator(
	verifier TokenVerifier,
	audience string,
) *OIDCAuthenticator {
	return &OIDCAuthenticator{
		verifier: verifier,
		audience: audience,
	}
}

// Authenticate extracts a Bearer token from the Authorization header,
// verifies it using the configured TokenVerifier, and returns the
// authenticated identity.
func (a *OIDCAuthenticator) Authenticate(
	r *http.Request,
) (*AuthInfo, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, ErrUnauthenticated
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, ErrUnauthenticated
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	claims, err := a.verifier.Verify(r.Context(), token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if a.audience != "" && !containsAudience(claims.Audience, a.audience) {
		return nil, fmt.Errorf(
			"%w: token audience %v does not contain %s",
			ErrInvalidToken,
			claims.Audience,
			a.audience,
		)
	}

	return &AuthInfo{
		Method:  AuthMethodOIDC,
		Subject: claims.Subject,
		Claims:  claims.Claims,
	}, nil
}

// Method returns the authentication method type.
func (a *OIDCAuthenticator) Method() AuthMethod {
	return AuthMethodOIDC
}

// containsAudience checks if the expected audience is present in the
// audience list.
func containsAudience(audiences []string, expected string) bool {
	for _, aud := range audiences {
		if aud == expected {
			return true
		}
	}
	return false
}
