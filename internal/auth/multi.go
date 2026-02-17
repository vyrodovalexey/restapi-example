package auth

import (
	"errors"
	"net/http"
)

// MultiAuthenticator tries multiple authenticators in order, returning
// the first successful result. If an authenticator returns
// ErrUnauthenticated (no credentials), the next authenticator is tried.
// If an authenticator returns any other error (invalid credentials),
// authentication fails immediately.
type MultiAuthenticator struct {
	authenticators []Authenticator
}

// NewMultiAuthenticator creates a new multi-method authenticator that
// tries each provided authenticator in order.
func NewMultiAuthenticator(
	authenticators ...Authenticator,
) *MultiAuthenticator {
	return &MultiAuthenticator{
		authenticators: authenticators,
	}
}

// Authenticate tries each configured authenticator in order.
// It returns the first successful authentication result.
// If all authenticators return ErrUnauthenticated, it returns
// ErrUnauthenticated. If any authenticator returns a different error
// (e.g., invalid credentials), that error is returned immediately.
func (a *MultiAuthenticator) Authenticate(
	r *http.Request,
) (*AuthInfo, error) {
	if len(a.authenticators) == 0 {
		return nil, ErrUnauthenticated
	}

	for _, authenticator := range a.authenticators {
		info, err := authenticator.Authenticate(r)
		if err == nil {
			return info, nil
		}

		// If the error is not "unauthenticated" (i.e., credentials
		// were provided but invalid), fail immediately.
		if !errors.Is(err, ErrUnauthenticated) {
			return nil, err
		}
	}

	return nil, ErrUnauthenticated
}

// Method returns the authentication method type.
func (a *MultiAuthenticator) Method() AuthMethod {
	return AuthMethodMulti
}
