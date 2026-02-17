// Package auth provides authentication for the REST API.
package auth

import (
	"context"
	"errors"
	"net/http"
)

// AuthMethod represents the authentication method used.
type AuthMethod string

const (
	// AuthMethodNone indicates no authentication.
	AuthMethodNone AuthMethod = "none"
	// AuthMethodMTLS indicates mutual TLS authentication.
	AuthMethodMTLS AuthMethod = "mtls"
	// AuthMethodOIDC indicates OpenID Connect authentication.
	AuthMethodOIDC AuthMethod = "oidc"
	// AuthMethodBasic indicates HTTP Basic authentication.
	AuthMethodBasic AuthMethod = "basic"
	// AuthMethodAPIKey indicates API key authentication.
	AuthMethodAPIKey AuthMethod = "apikey"
	// AuthMethodMulti indicates multi-method authentication.
	AuthMethodMulti AuthMethod = "multi"
)

// AuthInfo holds authenticated identity information.
type AuthInfo struct {
	Method  AuthMethod
	Subject string
	Claims  map[string]any
}

// Authenticator validates a request and returns auth info.
type Authenticator interface {
	Authenticate(r *http.Request) (*AuthInfo, error)
	Method() AuthMethod
}

// Sentinel errors for authentication failures.
var (
	ErrUnauthenticated    = errors.New("unauthenticated: no credentials provided")
	ErrInvalidToken       = errors.New("invalid token")
	ErrInvalidCert        = errors.New("invalid client certificate")
	ErrInvalidAPIKey      = errors.New("invalid API key")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// contextKey is the type for context keys in this package.
type contextKey string

// authInfoKey is the context key for AuthInfo.
const authInfoKey contextKey = "auth_info"

// FromContext retrieves AuthInfo from the context.
func FromContext(ctx context.Context) (*AuthInfo, bool) {
	info, ok := ctx.Value(authInfoKey).(*AuthInfo)
	return info, ok
}

// WithAuthInfo stores AuthInfo in the context.
func WithAuthInfo(ctx context.Context, info *AuthInfo) context.Context {
	return context.WithValue(ctx, authInfoKey, info)
}
