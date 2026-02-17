package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

// publicPaths are paths that don't require authentication.
var publicPaths = map[string]bool{
	"/health":  true,
	"/ready":   true,
	"/metrics": true,
}

// Auth returns a middleware that authenticates requests.
// Public paths (health, ready, metrics), CORS preflight requests,
// and WebSocket upgrade requests are excluded from authentication.
func Auth(
	authenticator auth.Authenticator,
	logger *zap.Logger,
) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			// Skip auth for public paths
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for CORS preflight
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for WebSocket upgrade
			if isWebSocketUpgrade(r) {
				next.ServeHTTP(w, r)
				return
			}

			info, err := authenticator.Authenticate(r)
			if err != nil {
				logger.Warn("authentication failed",
					zap.String("path", r.URL.Path),
					zap.String("method", r.Method),
					zap.String("remote_addr", r.RemoteAddr),
					zap.Error(err),
				)
				writeAuthError(w, err)
				return
			}

			logger.Debug("authentication successful",
				zap.String("subject", info.Subject),
				zap.String("method", string(info.Method)),
				zap.String("path", r.URL.Path),
			)

			ctx := auth.WithAuthInfo(r.Context(), info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isPublicPath checks whether the given path is a public path that
// does not require authentication. Matches exact public paths and
// their sub-paths (e.g. /health and /health/live), but rejects
// paths that merely share a prefix without a path separator
// (e.g. /healthXXX is not public).
func isPublicPath(path string) bool {
	if publicPaths[path] {
		return true
	}

	for p := range publicPaths {
		if strings.HasPrefix(path, p+"/") {
			return true
		}
	}

	return false
}

// isWebSocketUpgrade checks whether the request is a WebSocket
// upgrade request.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// authErrorResponse is the JSON error response for auth failures.
type authErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// writeAuthError writes an appropriate HTTP 401 response with
// WWW-Authenticate header based on the error type.
func writeAuthError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	setWWWAuthenticateHeader(w, err)

	w.WriteHeader(http.StatusUnauthorized)

	resp := authErrorResponse{
		Code:    http.StatusUnauthorized,
		Message: err.Error(),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// setWWWAuthenticateHeader sets the WWW-Authenticate header based on
// the authentication error type.
func setWWWAuthenticateHeader(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		w.Header().Set(
			"WWW-Authenticate", "Bearer, Basic, API-Key",
		)
	case errors.Is(err, auth.ErrInvalidToken):
		w.Header().Set(
			"WWW-Authenticate",
			`Bearer error="invalid_token"`,
		)
	case errors.Is(err, auth.ErrInvalidCredentials):
		w.Header().Set(
			"WWW-Authenticate", `Basic realm="restapi"`,
		)
	case errors.Is(err, auth.ErrInvalidAPIKey):
		w.Header().Set("WWW-Authenticate", "API-Key")
	case errors.Is(err, auth.ErrInvalidCert):
		w.Header().Set("WWW-Authenticate", "mTLS")
	}
}
