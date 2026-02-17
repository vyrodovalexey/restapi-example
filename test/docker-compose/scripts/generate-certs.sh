#!/bin/sh
# =============================================================================
# generate-certs.sh – Generate self-signed certificates for local testing
#
# This is a fallback script that creates a CA + server + client certificates
# using OpenSSL when Vault is not available. Useful for quick local testing
# without spinning up the full docker-compose environment.
#
# Usage:
#   ./generate-certs.sh [output-dir]
#
# Default output directory: ./certs
# =============================================================================
set -e

CERTS_DIR="${1:-./certs}"
CA_SUBJECT="/C=US/ST=Test/L=Test/O=RestAPI-Test/OU=Dev/CN=Test Root CA"
SERVER_CN="restapi-server"
CLIENT_CN="restapi-client"
DAYS=365
KEY_SIZE=2048

# ---------------------------------------------------------------------------
# Create output directory
# ---------------------------------------------------------------------------
mkdir -p "${CERTS_DIR}"
echo "==> Generating certificates in ${CERTS_DIR}"

# ---------------------------------------------------------------------------
# Step 1 – Generate CA key and certificate
# ---------------------------------------------------------------------------
generate_ca() {
  echo "==> Generating CA key and certificate ..."

  openssl genrsa -out "${CERTS_DIR}/ca-key.pem" ${KEY_SIZE} 2>/dev/null

  openssl req -new -x509 \
    -key "${CERTS_DIR}/ca-key.pem" \
    -out "${CERTS_DIR}/ca-cert.pem" \
    -days ${DAYS} \
    -subj "${CA_SUBJECT}" \
    2>/dev/null

  echo "    CA certificate: ${CERTS_DIR}/ca-cert.pem"
  echo "    CA key:         ${CERTS_DIR}/ca-key.pem"
}

# ---------------------------------------------------------------------------
# Step 2 – Generate server certificate
# ---------------------------------------------------------------------------
generate_server_cert() {
  echo "==> Generating server certificate (CN=${SERVER_CN}) ..."

  # Create server key
  openssl genrsa -out "${CERTS_DIR}/server-key.pem" ${KEY_SIZE} 2>/dev/null

  # Create CSR
  openssl req -new \
    -key "${CERTS_DIR}/server-key.pem" \
    -out "${CERTS_DIR}/server.csr" \
    -subj "/C=US/ST=Test/L=Test/O=RestAPI-Test/OU=Server/CN=${SERVER_CN}" \
    2>/dev/null

  # Create extensions file for SAN
  cat > "${CERTS_DIR}/server-ext.cnf" <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=@alt_names

[alt_names]
DNS.1 = ${SERVER_CN}
DNS.2 = localhost
IP.1 = 127.0.0.1
IP.2 = 0.0.0.0
EOF

  # Sign with CA
  openssl x509 -req \
    -in "${CERTS_DIR}/server.csr" \
    -CA "${CERTS_DIR}/ca-cert.pem" \
    -CAkey "${CERTS_DIR}/ca-key.pem" \
    -CAcreateserial \
    -out "${CERTS_DIR}/server-cert.pem" \
    -days ${DAYS} \
    -extfile "${CERTS_DIR}/server-ext.cnf" \
    2>/dev/null

  echo "    Server certificate: ${CERTS_DIR}/server-cert.pem"
  echo "    Server key:         ${CERTS_DIR}/server-key.pem"

  # Clean up temporary files
  rm -f "${CERTS_DIR}/server.csr" "${CERTS_DIR}/server-ext.cnf" "${CERTS_DIR}/ca-cert.srl"
}

# ---------------------------------------------------------------------------
# Step 3 – Generate client certificate
# ---------------------------------------------------------------------------
generate_client_cert() {
  local cn="$1"
  local prefix="$2"

  echo "==> Generating client certificate (CN=${cn}) ..."

  # Create client key
  openssl genrsa -out "${CERTS_DIR}/${prefix}-key.pem" ${KEY_SIZE} 2>/dev/null

  # Create CSR
  openssl req -new \
    -key "${CERTS_DIR}/${prefix}-key.pem" \
    -out "${CERTS_DIR}/${prefix}.csr" \
    -subj "/C=US/ST=Test/L=Test/O=RestAPI-Test/OU=Client/CN=${cn}" \
    2>/dev/null

  # Create extensions file
  cat > "${CERTS_DIR}/${prefix}-ext.cnf" <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
EOF

  # Sign with CA
  openssl x509 -req \
    -in "${CERTS_DIR}/${prefix}.csr" \
    -CA "${CERTS_DIR}/ca-cert.pem" \
    -CAkey "${CERTS_DIR}/ca-key.pem" \
    -CAcreateserial \
    -out "${CERTS_DIR}/${prefix}-cert.pem" \
    -days ${DAYS} \
    -extfile "${CERTS_DIR}/${prefix}-ext.cnf" \
    2>/dev/null

  echo "    Client certificate: ${CERTS_DIR}/${prefix}-cert.pem"
  echo "    Client key:         ${CERTS_DIR}/${prefix}-key.pem"

  # Clean up temporary files
  rm -f "${CERTS_DIR}/${prefix}.csr" "${CERTS_DIR}/${prefix}-ext.cnf" "${CERTS_DIR}/ca-cert.srl"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  echo "============================================="
  echo " Self-Signed Certificate Generation"
  echo "============================================="

  generate_ca
  generate_server_cert
  generate_client_cert "${CLIENT_CN}" "client"
  generate_client_cert "test-client" "test-client"

  # Set readable permissions
  chmod 644 "${CERTS_DIR}"/*.pem

  echo ""
  echo "============================================="
  echo " Certificate generation complete"
  echo "============================================="
  echo ""
  echo "Files in ${CERTS_DIR}:"
  ls -la "${CERTS_DIR}/"*.pem
  echo ""
  echo "Verify server cert:"
  openssl x509 -in "${CERTS_DIR}/server-cert.pem" -noout -subject -issuer -dates 2>/dev/null || true
}

main "$@"
