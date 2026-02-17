#!/bin/sh
# =============================================================================
# init-vault.sh – Initialise Vault PKI secrets engine and generate certificates
#
# This script is executed by the vault-init service after Vault is healthy.
# It is idempotent: re-running it will overwrite existing certificates but
# will not fail if the PKI engine is already enabled.
# =============================================================================
set -e

# ---------------------------------------------------------------------------
# Configuration (sourced from environment with sensible defaults)
# ---------------------------------------------------------------------------
VAULT_ADDR="${VAULT_ADDR:-http://vault:8200}"
VAULT_TOKEN="${VAULT_TOKEN:-myroot}"
PKI_PATH="${VAULT_PKI_PATH:-pki}"
PKI_ROLE="${VAULT_PKI_ROLE:-restapi-server}"
CERT_TTL="${CERT_TTL:-8760h}"
SERVER_CN="${SERVER_CN:-restapi-server}"
CLIENT_CN="${CLIENT_CN:-restapi-client}"
SERVER_ALT_NAMES="${SERVER_ALT_NAMES:-restapi-server,localhost}"
SERVER_IP_SANS="${SERVER_IP_SANS:-127.0.0.1}"
CERTS_DIR="${CERTS_DIR:-/certs}"

export VAULT_ADDR VAULT_TOKEN

# ---------------------------------------------------------------------------
# Helper: wait for Vault to be ready
# ---------------------------------------------------------------------------
wait_for_vault() {
  echo "==> Waiting for Vault at ${VAULT_ADDR} ..."
  retries=0
  max_retries=30
  while [ "$retries" -lt "$max_retries" ]; do
    if vault status > /dev/null 2>&1; then
      echo "==> Vault is ready"
      return 0
    fi
    retries=$((retries + 1))
    echo "    attempt ${retries}/${max_retries} – retrying in 2s ..."
    sleep 2
  done
  echo "ERROR: Vault did not become ready in time"
  exit 1
}

# ---------------------------------------------------------------------------
# Step 1 – Enable PKI secrets engine
# ---------------------------------------------------------------------------
enable_pki() {
  echo "==> Enabling PKI secrets engine at ${PKI_PATH} ..."
  # Enable only if not already mounted
  if vault secrets list -format=json | grep -q "\"${PKI_PATH}/\""; then
    echo "    PKI engine already enabled at ${PKI_PATH}/"
  else
    vault secrets enable -path="${PKI_PATH}" pki
    echo "    PKI engine enabled"
  fi

  # Tune max lease TTL (must be >= CA TTL of 87600h / 10 years)
  vault secrets tune -max-lease-ttl="87600h" "${PKI_PATH}/"
  echo "    Max lease TTL set to 87600h"
}

# ---------------------------------------------------------------------------
# Step 2 – Generate root CA
# ---------------------------------------------------------------------------
generate_root_ca() {
  echo "==> Generating root CA ..."
  # Use a CA TTL that is longer than the certificate TTL to avoid
  # "notAfter beyond CA expiration" errors.
  vault write -format=json "${PKI_PATH}/root/generate/internal" \
    common_name="Test Root CA" \
    ttl="87600h" \
    key_type="rsa" \
    key_bits=4096 \
    | tee /tmp/root_ca.json > /dev/null

  # Extract CA certificate
  vault read -field=certificate "${PKI_PATH}/cert/ca" > "${CERTS_DIR}/ca-cert.pem"
  echo "    Root CA written to ${CERTS_DIR}/ca-cert.pem"
}

# ---------------------------------------------------------------------------
# Step 3 – Configure PKI URLs
# ---------------------------------------------------------------------------
configure_urls() {
  echo "==> Configuring PKI URLs ..."
  vault write "${PKI_PATH}/config/urls" \
    issuing_certificates="${VAULT_ADDR}/v1/${PKI_PATH}/ca" \
    crl_distribution_points="${VAULT_ADDR}/v1/${PKI_PATH}/crl"
  echo "    URLs configured"
}

# ---------------------------------------------------------------------------
# Step 4 – Create roles for server and client certificates
# ---------------------------------------------------------------------------
create_roles() {
  echo "==> Creating PKI role '${PKI_ROLE}' for server certificates ..."
  vault write "${PKI_PATH}/roles/${PKI_ROLE}" \
    allowed_domains="${SERVER_CN},localhost" \
    allow_subdomains=true \
    allow_bare_domains=true \
    allow_localhost=true \
    allow_ip_sans=true \
    server_flag=true \
    client_flag=false \
    max_ttl="${CERT_TTL}" \
    key_type="rsa" \
    key_bits=2048
  echo "    Server role created"

  echo "==> Creating PKI role 'restapi-client' for client certificates ..."
  vault write "${PKI_PATH}/roles/restapi-client" \
    allowed_domains="restapi-client,localhost,test-client" \
    allow_subdomains=true \
    allow_bare_domains=true \
    allow_localhost=true \
    allow_ip_sans=true \
    server_flag=false \
    client_flag=true \
    max_ttl="${CERT_TTL}" \
    key_type="rsa" \
    key_bits=2048
  echo "    Client role created"
}

# ---------------------------------------------------------------------------
# Step 5 – Issue server certificate
# ---------------------------------------------------------------------------
issue_server_cert() {
  echo "==> Issuing server certificate (CN=${SERVER_CN}) ..."
  vault write -format=json "${PKI_PATH}/issue/${PKI_ROLE}" \
    common_name="${SERVER_CN}" \
    alt_names="${SERVER_ALT_NAMES}" \
    ip_sans="${SERVER_IP_SANS}" \
    ttl="${CERT_TTL}" \
    > /tmp/server_cert.json

  # Extract certificate, key and CA chain
  cat /tmp/server_cert.json | vault kv get -format=json - 2>/dev/null || true
  python3 -c "
import json, sys
data = json.load(open('/tmp/server_cert.json'))['data']
open('${CERTS_DIR}/server-cert.pem', 'w').write(data['certificate'] + '\n')
open('${CERTS_DIR}/server-key.pem', 'w').write(data['private_key'] + '\n')
if data.get('issuing_ca'):
    open('${CERTS_DIR}/server-ca.pem', 'w').write(data['issuing_ca'] + '\n')
" 2>/dev/null || {
    # Fallback: use jq-style parsing with sed/grep if python is unavailable
    # Extract certificate
    sed -n '/"certificate"/,/-----END CERTIFICATE-----/p' /tmp/server_cert.json \
      | sed 's/.*"certificate": "//;s/",$//' \
      | sed 's/\\n/\n/g' > "${CERTS_DIR}/server-cert.pem"
    # Extract private key
    sed -n '/"private_key"/,/-----END RSA PRIVATE KEY-----/p' /tmp/server_cert.json \
      | sed 's/.*"private_key": "//;s/",$//' \
      | sed 's/\\n/\n/g' > "${CERTS_DIR}/server-key.pem"
    # Extract issuing CA
    sed -n '/"issuing_ca"/,/-----END CERTIFICATE-----/p' /tmp/server_cert.json \
      | sed 's/.*"issuing_ca": "//;s/",$//' \
      | sed 's/\\n/\n/g' > "${CERTS_DIR}/server-ca.pem"
  }

  echo "    Server certificate written to ${CERTS_DIR}/server-cert.pem"
  echo "    Server key written to ${CERTS_DIR}/server-key.pem"
}

# ---------------------------------------------------------------------------
# Step 6 – Issue client certificates for testing
# ---------------------------------------------------------------------------
issue_client_cert() {
  local cn="$1"
  local filename="$2"

  echo "==> Issuing client certificate (CN=${cn}) ..."
  vault write -format=json "${PKI_PATH}/issue/restapi-client" \
    common_name="${cn}" \
    ttl="${CERT_TTL}" \
    > "/tmp/${filename}.json"

  python3 -c "
import json
data = json.load(open('/tmp/${filename}.json'))['data']
open('${CERTS_DIR}/${filename}-cert.pem', 'w').write(data['certificate'] + '\n')
open('${CERTS_DIR}/${filename}-key.pem', 'w').write(data['private_key'] + '\n')
" 2>/dev/null || {
    sed -n '/"certificate"/,/-----END CERTIFICATE-----/p' "/tmp/${filename}.json" \
      | sed 's/.*"certificate": "//;s/",$//' \
      | sed 's/\\n/\n/g' > "${CERTS_DIR}/${filename}-cert.pem"
    sed -n '/"private_key"/,/-----END RSA PRIVATE KEY-----/p' "/tmp/${filename}.json" \
      | sed 's/.*"private_key": "//;s/",$//' \
      | sed 's/\\n/\n/g' > "${CERTS_DIR}/${filename}-key.pem"
  }

  echo "    Client certificate written to ${CERTS_DIR}/${filename}-cert.pem"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  echo "============================================="
  echo " Vault PKI Initialisation"
  echo "============================================="

  wait_for_vault
  enable_pki
  generate_root_ca
  configure_urls
  create_roles
  issue_server_cert
  issue_client_cert "${CLIENT_CN}" "client"
  issue_client_cert "test-client" "test-client"

  # Set readable permissions on certificate files
  chmod 644 "${CERTS_DIR}"/*.pem 2>/dev/null || true

  echo ""
  echo "============================================="
  echo " Vault PKI initialisation complete"
  echo "============================================="
  echo ""
  echo "Certificates in ${CERTS_DIR}:"
  ls -la "${CERTS_DIR}/"
}

main "$@"
