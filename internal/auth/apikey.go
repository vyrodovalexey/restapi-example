package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

// APIKeyHeader is the HTTP header name for API key authentication.
const APIKeyHeader = "X-API-Key"

// APIKeyAuthenticator authenticates requests using API keys provided
// in the X-API-Key header with constant-time comparison.
type APIKeyAuthenticator struct {
	keys map[string]string // key value -> key name
}

// NewAPIKeyAuthenticator creates a new API key authenticator from a
// configuration string in the format "key1:name1,key2:name2".
// Each entry must contain exactly one colon separating the key value
// from the key name.
func NewAPIKeyAuthenticator(
	keysConfig string,
) (*APIKeyAuthenticator, error) {
	trimmed := strings.TrimSpace(keysConfig)
	if trimmed == "" {
		return nil, fmt.Errorf(
			"apikey auth: keys config must not be empty",
		)
	}

	keys := make(map[string]string)
	entries := strings.Split(trimmed, ",")

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf(
				"apikey auth: invalid entry format, expected key:name",
			)
		}

		key := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])

		if key == "" || name == "" {
			return nil, fmt.Errorf(
				"apikey auth: key and name must not be empty",
			)
		}

		keys[key] = name
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf(
			"apikey auth: no valid key entries found",
		)
	}

	return &APIKeyAuthenticator{keys: keys}, nil
}

// Authenticate extracts the API key from the X-API-Key header and
// validates it against the configured keys using constant-time
// comparison to prevent timing attacks.
func (a *APIKeyAuthenticator) Authenticate(
	r *http.Request,
) (*AuthInfo, error) {
	apiKey := r.Header.Get(APIKeyHeader)
	if apiKey == "" {
		return nil, ErrUnauthenticated
	}

	for key, name := range a.keys {
		if subtle.ConstantTimeCompare(
			[]byte(apiKey), []byte(key),
		) == 1 {
			return &AuthInfo{
				Method:  AuthMethodAPIKey,
				Subject: name,
			}, nil
		}
	}

	return nil, ErrInvalidAPIKey
}

// Method returns the authentication method type.
func (a *APIKeyAuthenticator) Method() AuthMethod {
	return AuthMethodAPIKey
}
