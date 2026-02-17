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
	users     map[string]string // username -> bcrypt hash
	dummyHash []byte            // pre-computed hash used when user is not found to prevent timing leaks
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

	// Generate a dummy bcrypt hash at init time. This hash is compared against
	// when the requested user does not exist, ensuring constant-time behavior
	// regardless of whether the user is known.
	dummyHash, err := bcrypt.GenerateFromPassword(
		[]byte("dummy-password-for-timing-safety"), bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, fmt.Errorf("basic auth: generating dummy hash: %w", err)
	}

	return &BasicAuthenticator{users: users, dummyHash: dummyHash}, nil
}

// Authenticate extracts Basic auth credentials from the request,
// looks up the user, and verifies the password against the stored
// bcrypt hash. When the user is not found, a comparison against a
// dummy hash is performed to prevent timing-based user enumeration.
func (a *BasicAuthenticator) Authenticate(
	r *http.Request,
) (*AuthInfo, error) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return nil, ErrUnauthenticated
	}

	hash, exists := a.users[username]
	if !exists {
		// Compare against dummy hash to consume the same time as a real
		// comparison, preventing timing-based user enumeration.
		_ = bcrypt.CompareHashAndPassword(a.dummyHash, []byte(password))
		return nil, fmt.Errorf("%w: invalid username or password", ErrInvalidCredentials)
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(hash), []byte(password),
	); err != nil {
		return nil, fmt.Errorf(
			"%w: invalid username or password", ErrInvalidCredentials,
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
