package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

// mockAuthenticator is a test double for auth.Authenticator.
type mockAuthenticator struct {
	info   *auth.AuthInfo
	err    error
	method auth.AuthMethod
	called bool
}

func (m *mockAuthenticator) Authenticate(
	_ *http.Request,
) (*auth.AuthInfo, error) {
	m.called = true
	return m.info, m.err
}

func (m *mockAuthenticator) Method() auth.AuthMethod {
	return m.method
}

func TestMultiAuthenticator_Authenticate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		authenticators func() []*mockAuthenticator
		wantSubject    string
		wantMethod     auth.AuthMethod
		wantErr        bool
		wantErrIs      error
		checkCalled    func(t *testing.T, auths []*mockAuthenticator)
	}{
		{
			name: "empty authenticator list returns ErrUnauthenticated",
			authenticators: func() []*mockAuthenticator {
				return nil
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name: "single authenticator succeeds",
			authenticators: func() []*mockAuthenticator {
				return []*mockAuthenticator{
					{
						info: &auth.AuthInfo{
							Method:  auth.AuthMethodBasic,
							Subject: "user1",
						},
						method: auth.AuthMethodBasic,
					},
				}
			},
			wantErr:     false,
			wantSubject: "user1",
			wantMethod:  auth.AuthMethodBasic,
		},
		{
			name: "single authenticator fails returns error",
			authenticators: func() []*mockAuthenticator {
				return []*mockAuthenticator{
					{
						err:    auth.ErrInvalidToken,
						method: auth.AuthMethodOIDC,
					},
				}
			},
			wantErr:   true,
			wantErrIs: auth.ErrInvalidToken,
		},
		{
			name: "first succeeds second not tried",
			authenticators: func() []*mockAuthenticator {
				return []*mockAuthenticator{
					{
						info: &auth.AuthInfo{
							Method:  auth.AuthMethodAPIKey,
							Subject: "svc-a",
						},
						method: auth.AuthMethodAPIKey,
					},
					{
						info: &auth.AuthInfo{
							Method:  auth.AuthMethodBasic,
							Subject: "user2",
						},
						method: auth.AuthMethodBasic,
					},
				}
			},
			wantErr:     false,
			wantSubject: "svc-a",
			wantMethod:  auth.AuthMethodAPIKey,
			checkCalled: func(t *testing.T, auths []*mockAuthenticator) {
				t.Helper()
				if !auths[0].called {
					t.Error("first authenticator should be called")
				}
				if auths[1].called {
					t.Error("second authenticator should NOT be called")
				}
			},
		},
		{
			name: "first returns ErrUnauthenticated second succeeds",
			authenticators: func() []*mockAuthenticator {
				return []*mockAuthenticator{
					{
						err:    auth.ErrUnauthenticated,
						method: auth.AuthMethodOIDC,
					},
					{
						info: &auth.AuthInfo{
							Method:  auth.AuthMethodBasic,
							Subject: "user-from-basic",
						},
						method: auth.AuthMethodBasic,
					},
				}
			},
			wantErr:     false,
			wantSubject: "user-from-basic",
			wantMethod:  auth.AuthMethodBasic,
			checkCalled: func(t *testing.T, auths []*mockAuthenticator) {
				t.Helper()
				if !auths[0].called {
					t.Error("first authenticator should be called")
				}
				if !auths[1].called {
					t.Error("second authenticator should be called")
				}
			},
		},
		{
			name: "first returns ErrInvalidToken fails immediately",
			authenticators: func() []*mockAuthenticator {
				return []*mockAuthenticator{
					{
						err:    auth.ErrInvalidToken,
						method: auth.AuthMethodOIDC,
					},
					{
						info: &auth.AuthInfo{
							Method:  auth.AuthMethodBasic,
							Subject: "user-from-basic",
						},
						method: auth.AuthMethodBasic,
					},
				}
			},
			wantErr:   true,
			wantErrIs: auth.ErrInvalidToken,
			checkCalled: func(t *testing.T, auths []*mockAuthenticator) {
				t.Helper()
				if !auths[0].called {
					t.Error("first authenticator should be called")
				}
				if auths[1].called {
					t.Error("second authenticator should NOT be called")
				}
			},
		},
		{
			name: "all return ErrUnauthenticated returns ErrUnauthenticated",
			authenticators: func() []*mockAuthenticator {
				return []*mockAuthenticator{
					{
						err:    auth.ErrUnauthenticated,
						method: auth.AuthMethodOIDC,
					},
					{
						err:    auth.ErrUnauthenticated,
						method: auth.AuthMethodBasic,
					},
					{
						err:    auth.ErrUnauthenticated,
						method: auth.AuthMethodAPIKey,
					},
				}
			},
			wantErr:   true,
			wantErrIs: auth.ErrUnauthenticated,
		},
		{
			name: "three authenticators middle one succeeds",
			authenticators: func() []*mockAuthenticator {
				return []*mockAuthenticator{
					{
						err:    auth.ErrUnauthenticated,
						method: auth.AuthMethodOIDC,
					},
					{
						info: &auth.AuthInfo{
							Method:  auth.AuthMethodAPIKey,
							Subject: "middle-svc",
						},
						method: auth.AuthMethodAPIKey,
					},
					{
						info: &auth.AuthInfo{
							Method:  auth.AuthMethodBasic,
							Subject: "last-user",
						},
						method: auth.AuthMethodBasic,
					},
				}
			},
			wantErr:     false,
			wantSubject: "middle-svc",
			wantMethod:  auth.AuthMethodAPIKey,
			checkCalled: func(t *testing.T, auths []*mockAuthenticator) {
				t.Helper()
				if !auths[0].called {
					t.Error("first authenticator should be called")
				}
				if !auths[1].called {
					t.Error("second authenticator should be called")
				}
				if auths[2].called {
					t.Error("third authenticator should NOT be called")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			mocks := tt.authenticators()
			auths := make([]auth.Authenticator, len(mocks))
			for i, m := range mocks {
				auths[i] = m
			}
			multi := auth.NewMultiAuthenticator(auths...)
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			// Act
			info, err := multi.Authenticate(req)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Fatal("Authenticate() error = nil, want error")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf(
						"Authenticate() error = %v, want errors.Is %v",
						err, tt.wantErrIs,
					)
				}
			} else {
				if err != nil {
					t.Fatalf("Authenticate() error = %v, want nil", err)
				}
				if info == nil {
					t.Fatal("Authenticate() returned nil AuthInfo")
				}
				if info.Subject != tt.wantSubject {
					t.Errorf(
						"Subject = %q, want %q",
						info.Subject, tt.wantSubject,
					)
				}
				if info.Method != tt.wantMethod {
					t.Errorf(
						"Method = %q, want %q",
						info.Method, tt.wantMethod,
					)
				}
			}

			if tt.checkCalled != nil {
				tt.checkCalled(t, mocks)
			}
		})
	}
}

func TestMultiAuthenticator_Method(t *testing.T) {
	t.Parallel()

	multi := auth.NewMultiAuthenticator()

	if multi.Method() != auth.AuthMethodMulti {
		t.Errorf("Method() = %q, want %q", multi.Method(), auth.AuthMethodMulti)
	}
}
