package auth_test

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vyrodovalexey/restapi-example/internal/auth"
)

func createTestCert(
	cn string,
	orgs []string,
	dnsNames []string,
) *x509.Certificate {
	return &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: orgs,
		},
		DNSNames: dnsNames,
	}
}

func TestMTLSAuthenticator_Authenticate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupReq    func() *http.Request
		wantSubject string
		wantClaims  map[string]any
		wantErr     error
		wantErrNil  bool
	}{
		{
			name: "nil TLS state returns ErrUnauthenticated",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = nil
				return req
			},
			wantErr: auth.ErrUnauthenticated,
		},
		{
			name: "empty PeerCertificates returns ErrInvalidCert",
			setupReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{},
				}
				return req
			},
			wantErr: auth.ErrInvalidCert,
		},
		{
			name: "valid client cert returns AuthInfo with correct CN",
			setupReq: func() *http.Request {
				cert := createTestCert("client.example.com", nil, nil)
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{cert},
				}
				return req
			},
			wantErrNil:  true,
			wantSubject: "client.example.com",
			wantClaims:  map[string]any{},
		},
		{
			name: "extracts Organization into Claims",
			setupReq: func() *http.Request {
				cert := createTestCert(
					"client.example.com",
					[]string{"Org1", "Org2"},
					nil,
				)
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{cert},
				}
				return req
			},
			wantErrNil:  true,
			wantSubject: "client.example.com",
			wantClaims: map[string]any{
				"organizations": []string{"Org1", "Org2"},
			},
		},
		{
			name: "extracts DNSNames into Claims",
			setupReq: func() *http.Request {
				cert := createTestCert(
					"client.example.com",
					nil,
					[]string{"dns1.example.com", "dns2.example.com"},
				)
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{cert},
				}
				return req
			},
			wantErrNil:  true,
			wantSubject: "client.example.com",
			wantClaims: map[string]any{
				"dns_names": []string{
					"dns1.example.com",
					"dns2.example.com",
				},
			},
		},
		{
			name: "multiple peer certs uses first cert",
			setupReq: func() *http.Request {
				cert1 := createTestCert("first.example.com", nil, nil)
				cert2 := createTestCert("second.example.com", nil, nil)
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{cert1, cert2},
				}
				return req
			},
			wantErrNil:  true,
			wantSubject: "first.example.com",
			wantClaims:  map[string]any{},
		},
		{
			name: "cert with empty CN returns AuthInfo with empty Subject",
			setupReq: func() *http.Request {
				cert := createTestCert("", nil, nil)
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{cert},
				}
				return req
			},
			wantErrNil:  true,
			wantSubject: "",
			wantClaims:  map[string]any{},
		},
		{
			name: "cert with both orgs and DNS names",
			setupReq: func() *http.Request {
				cert := createTestCert(
					"full.example.com",
					[]string{"MyOrg"},
					[]string{"alt.example.com"},
				)
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{cert},
				}
				return req
			},
			wantErrNil:  true,
			wantSubject: "full.example.com",
			wantClaims: map[string]any{
				"organizations": []string{"MyOrg"},
				"dns_names":     []string{"alt.example.com"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			authenticator := auth.NewMTLSAuthenticator()
			req := tt.setupReq()

			// Act
			info, err := authenticator.Authenticate(req)

			// Assert
			if tt.wantErrNil {
				if err != nil {
					t.Fatalf("Authenticate() error = %v, want nil", err)
				}
				if info == nil {
					t.Fatal("Authenticate() returned nil AuthInfo")
				}
				if info.Method != auth.AuthMethodMTLS {
					t.Errorf("Method = %q, want %q", info.Method, auth.AuthMethodMTLS)
				}
				if info.Subject != tt.wantSubject {
					t.Errorf("Subject = %q, want %q", info.Subject, tt.wantSubject)
				}
				// Verify claims
				for key, wantVal := range tt.wantClaims {
					gotVal, exists := info.Claims[key]
					if !exists {
						t.Errorf("Claims[%q] not found", key)
						continue
					}
					assertSliceEqual(t, key, gotVal, wantVal)
				}
				// Verify no extra claims beyond expected
				for key := range info.Claims {
					if _, expected := tt.wantClaims[key]; !expected {
						t.Errorf("unexpected claim %q in AuthInfo", key)
					}
				}
			} else {
				if err != tt.wantErr {
					t.Errorf("Authenticate() error = %v, want %v", err, tt.wantErr)
				}
				if info != nil {
					t.Errorf("Authenticate() returned %v, want nil", info)
				}
			}
		})
	}
}

func assertSliceEqual(t *testing.T, key string, got, want any) {
	t.Helper()

	gotSlice, gotOK := got.([]string)
	wantSlice, wantOK := want.([]string)

	if !gotOK || !wantOK {
		t.Errorf("Claims[%q]: type mismatch got=%T want=%T", key, got, want)
		return
	}

	if len(gotSlice) != len(wantSlice) {
		t.Errorf("Claims[%q] len = %d, want %d", key, len(gotSlice), len(wantSlice))
		return
	}

	for i := range gotSlice {
		if gotSlice[i] != wantSlice[i] {
			t.Errorf("Claims[%q][%d] = %q, want %q", key, i, gotSlice[i], wantSlice[i])
		}
	}
}

func TestMTLSAuthenticator_Method(t *testing.T) {
	t.Parallel()

	authenticator := auth.NewMTLSAuthenticator()

	if authenticator.Method() != auth.AuthMethodMTLS {
		t.Errorf("Method() = %q, want %q", authenticator.Method(), auth.AuthMethodMTLS)
	}
}
