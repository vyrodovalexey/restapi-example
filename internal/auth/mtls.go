package auth

import "net/http"

// MTLSAuthenticator authenticates requests using mutual TLS client certificates.
type MTLSAuthenticator struct{}

// NewMTLSAuthenticator creates a new mTLS authenticator.
func NewMTLSAuthenticator() *MTLSAuthenticator {
	return &MTLSAuthenticator{}
}

// Authenticate validates the client certificate from the TLS connection
// and extracts identity information from the certificate subject.
func (a *MTLSAuthenticator) Authenticate(r *http.Request) (*AuthInfo, error) {
	if r.TLS == nil {
		return nil, ErrUnauthenticated
	}

	if len(r.TLS.PeerCertificates) == 0 {
		return nil, ErrInvalidCert
	}

	cert := r.TLS.PeerCertificates[0]
	cn := cert.Subject.CommonName

	claims := make(map[string]any)
	if len(cert.Subject.Organization) > 0 {
		claims["organizations"] = cert.Subject.Organization
	}
	if len(cert.DNSNames) > 0 {
		claims["dns_names"] = cert.DNSNames
	}

	return &AuthInfo{
		Method:  AuthMethodMTLS,
		Subject: cn,
		Claims:  claims,
	}, nil
}

// Method returns the authentication method type.
func (a *MTLSAuthenticator) Method() AuthMethod {
	return AuthMethodMTLS
}
