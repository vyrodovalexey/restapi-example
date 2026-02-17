#!/bin/sh
# =============================================================================
# wait-for-services.sh – Wait for all test environment services to be healthy
#
# Verifies that Vault PKI is configured, Keycloak realm exists, and the
# REST API server is accepting connections.
#
# Usage:
#   ./wait-for-services.sh
#
# Environment variables:
#   VAULT_ADDR       – Vault address    (default: http://localhost:8200)
#   VAULT_TOKEN      – Vault token      (default: myroot)
#   KEYCLOAK_URL     – Keycloak URL     (default: http://localhost:8090)
#   APP_HOST         – REST API host    (default: localhost)
#   APP_PORT         – REST API port    (default: 8080)
#   TIMEOUT          – Max wait in seconds (default: 120)
# =============================================================================
set -e

VAULT_ADDR="${VAULT_ADDR:-http://localhost:8200}"
VAULT_TOKEN="${VAULT_TOKEN:-myroot}"
KEYCLOAK_URL="${KEYCLOAK_URL:-http://localhost:8090}"
APP_HOST="${APP_HOST:-localhost}"
APP_PORT="${APP_PORT:-8080}"
TIMEOUT="${TIMEOUT:-120}"
KC_REALM="${KC_REALM:-restapi-test}"

export VAULT_ADDR VAULT_TOKEN

SECONDS_WAITED=0

# ---------------------------------------------------------------------------
# Helper: check with timeout
# ---------------------------------------------------------------------------
check_with_timeout() {
  local name="$1"
  local check_cmd="$2"
  local interval="${3:-3}"

  echo "==> Waiting for ${name} ..."
  while [ "${SECONDS_WAITED}" -lt "${TIMEOUT}" ]; do
    if eval "${check_cmd}" > /dev/null 2>&1; then
      echo "    ${name} is ready"
      return 0
    fi
    SECONDS_WAITED=$((SECONDS_WAITED + interval))
    echo "    ${name} not ready – waited ${SECONDS_WAITED}s / ${TIMEOUT}s"
    sleep "${interval}"
  done

  echo "ERROR: ${name} did not become ready within ${TIMEOUT}s"
  return 1
}

# ---------------------------------------------------------------------------
# Check 1 – Vault is unsealed and healthy
# ---------------------------------------------------------------------------
check_vault() {
  check_with_timeout "Vault" \
    "curl -sf '${VAULT_ADDR}/v1/sys/health' | grep -q '\"sealed\":false'"
}

# ---------------------------------------------------------------------------
# Check 2 – Vault PKI is configured (CA cert exists)
# ---------------------------------------------------------------------------
check_vault_pki() {
  check_with_timeout "Vault PKI" \
    "curl -sf -H 'X-Vault-Token: ${VAULT_TOKEN}' '${VAULT_ADDR}/v1/pki/ca/pem' | grep -q 'BEGIN CERTIFICATE'"
}

# ---------------------------------------------------------------------------
# Check 3 – Keycloak is healthy
# ---------------------------------------------------------------------------
check_keycloak() {
  check_with_timeout "Keycloak" \
    "curl -sf '${KEYCLOAK_URL}/realms/master/.well-known/openid-configuration' | grep -q 'issuer'"
}

# ---------------------------------------------------------------------------
# Check 4 – Keycloak realm exists
# ---------------------------------------------------------------------------
check_keycloak_realm() {
  check_with_timeout "Keycloak realm '${KC_REALM}'" \
    "curl -sf '${KEYCLOAK_URL}/realms/${KC_REALM}/.well-known/openid-configuration' | grep -q 'issuer'"
}

# ---------------------------------------------------------------------------
# Check 5 – REST API server is responding to health checks
# ---------------------------------------------------------------------------
check_restapi_server() {
  check_with_timeout "REST API server (${APP_HOST}:${APP_PORT})" \
    "curl -sf http://${APP_HOST}:${APP_PORT}/health | grep -q 'ok\|UP\|healthy'" \
    2
}

# ---------------------------------------------------------------------------
# Check 6 – Certificates exist on disk
# ---------------------------------------------------------------------------
check_certificates() {
  local certs_dir="${CERTS_DIR:-./certs}"
  echo "==> Checking certificates in ${certs_dir} ..."

  local missing=0
  for f in ca-cert.pem server-cert.pem server-key.pem client-cert.pem client-key.pem; do
    if [ -f "${certs_dir}/${f}" ]; then
      echo "    [OK] ${f}"
    else
      echo "    [MISSING] ${f}"
      missing=$((missing + 1))
    fi
  done

  if [ "${missing}" -gt 0 ]; then
    echo "    WARNING: ${missing} certificate file(s) missing"
    return 1
  fi
  echo "    All certificate files present"
  return 0
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  echo "============================================="
  echo " Waiting for Test Environment Services"
  echo "============================================="
  echo " Timeout: ${TIMEOUT}s"
  echo ""

  local failed=0

  check_vault || failed=$((failed + 1))
  check_vault_pki || failed=$((failed + 1))
  check_keycloak || failed=$((failed + 1))
  check_keycloak_realm || failed=$((failed + 1))
  check_restapi_server || failed=$((failed + 1))

  echo ""
  echo "============================================="
  if [ "${failed}" -gt 0 ]; then
    echo " ${failed} service check(s) FAILED"
    echo "============================================="
    exit 1
  else
    echo " All services are ready"
    echo "============================================="
    exit 0
  fi
}

main "$@"
