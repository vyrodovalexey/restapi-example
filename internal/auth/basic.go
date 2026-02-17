package auth

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// BasicAuthenticator authenticates requests using HTTP Basic authentication
// with bcrypt-hashed passwords.
type BasicAuthenticator struct {
	users map[string]string // username -> bcrypt hash
}

// NewBasicAuthenticator creates a new Basic authenticator from a
// configuration string in the format "user1:hash1,user2:hash2".
// Each entry must contain exactly one colon separating the username
// from the bcrypt hash.
func NewBasicAuthenticator(
	usersConfig string,
) (*BasicAuthenticator, error) {
	trimmed := strings.TrimSpace(usersConfig)
	if trimmed == "" {
		return nil, fmt.Errorf(
			"basic auth: users config must not be empty",
		)
	}

	users := make(map[string]string)
	entries := strings.Split(trimmed, ",")

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Find the first colon to split username from bcrypt hash.
		// Bcrypt hashes contain '$' but not additional colons in the
		// user:hash format.
		idx := strings.Index(entry, ":")
		if idx < 0 {
			return nil, fmt.Errorf(
				"basic auth: invalid entry format, expected user:hash",
			)
		}

		username := entry[:idx]
		hash := entry[idx+1:]

		if username == "" || hash == "" {
			return nil, fmt.Errorf(
				"basic auth: username and hash must not be empty",
			)
		}

		users[username] = hash
	}

	if len(users) == 0 {
		return nil, fmt.Errorf(
			"basic auth: no valid user entries found",
		)
	}

	return &BasicAuthenticator{users: users}, nil
}

// Authenticate extracts Basic auth credentials from the request,
// looks up the user, and verifies the password against the stored
// bcrypt hash.
func (a *BasicAuthenticator) Authenticate(
	r *http.Request,
) (*AuthInfo, error) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return nil, ErrUnauthenticated
	}

	hash, exists := a.users[username]
	if !exists {
		return nil, fmt.Errorf("%w: unknown user", ErrInvalidCredentials)
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(hash), []byte(password),
	); err != nil {
		return nil, fmt.Errorf(
			"%w: wrong password", ErrInvalidCredentials,
		)
	}

	return &AuthInfo{
		Method:  AuthMethodBasic,
		Subject: username,
	}, nil
}

// Method returns the authentication method type.
func (a *BasicAuthenticator) Method() AuthMethod {
	return AuthMethodBasic
}
